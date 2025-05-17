package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"messaging-app/internal/kafka"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MessageService struct {
	messageRepo    *repositories.MessageRepository
	groupRepo      *repositories.GroupRepository
	friendshipRepo *repositories.FriendshipRepository
	producer       *kafka.MessageProducer
	redisClient    *redis.ClusterClient
}

func NewMessageService(
	messageRepo *repositories.MessageRepository,
	groupRepo *repositories.GroupRepository,
	friendshipRepo *repositories.FriendshipRepository,
	producer *kafka.MessageProducer,
	redisClient *redis.ClusterClient,
) *MessageService {
	return &MessageService{
		messageRepo:    messageRepo,
		groupRepo:      groupRepo,
		friendshipRepo: friendshipRepo,
		producer:       producer,
		redisClient:    redisClient,
	}
}

func (s *MessageService) SendMessage(ctx context.Context, senderID primitive.ObjectID, req models.MessageRequest) (*models.Message, error) {
	msg := &models.Message{
		SenderID:    senderID,
		Content:     req.Content,
		ContentType: req.ContentType,
		MediaURLs:   req.MediaURLs,
	}

	if req.GroupID != "" {
		return s.handleGroupMessage(ctx, msg, req.GroupID)
	}
	return s.handleDirectMessage(ctx, msg, req.ReceiverID)
}

func (s *MessageService) handleGroupMessage(ctx context.Context, msg *models.Message, groupID string) (*models.Message, error) {
	gID, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		return nil, errors.New("invalid group ID")
	}

	// Check group membership using Redis cache first
	cacheKey := "group:" + groupID + ":members"
	members, err := s.redisClient.SMembers(ctx, cacheKey).Result()
	if err == nil && len(members) > 0 {
		// Check cache
		found := false
		for _, m := range members {
			if m == msg.SenderID.Hex() {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("not a group member")
		}
	} else {
		// Fallback to database
		group, err := s.groupRepo.GetGroup(ctx, gID)
		if err != nil {
			return nil, err
		}

		isMember := false
		for _, m := range group.Members {
			if m == msg.SenderID {
				isMember = true
				break
			}
		}

		if !isMember {
			return nil, errors.New("not a group member")
		}
	}

	msg.GroupID = gID
	
	// Get group name from cache or DB
	groupName, err := s.redisClient.Get(ctx, "group:"+groupID+":name").Result()
	if err != nil {
		group, err := s.groupRepo.GetGroup(ctx, gID)
		if err != nil {
			return nil, err
		}
		groupName = group.Name
		s.redisClient.Set(ctx, "group:"+groupID+":name", groupName, 24*time.Hour)
	}
	msg.GroupName = groupName

	// Get sender info from cache or DB
	senderName, err := s.redisClient.Get(ctx, "user:"+msg.SenderID.Hex()+":name").Result()
	if err != nil {
		// In a real app, you'd fetch from user repo
		senderName = "Unknown"
	}
	msg.SenderName = senderName

	// Save to database
	createdMsg, err := s.messageRepo.CreateMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	// Publish to Kafka
	if err := s.producer.ProduceMessage(ctx, *createdMsg); err != nil {
		// Log error but don't fail the operation
		log.Printf("Failed to produce message to Kafka: %v", err)
	}

	return createdMsg, nil
}

func (s *MessageService) handleDirectMessage(ctx context.Context, msg *models.Message, receiverID string) (*models.Message, error) {
	rID, err := primitive.ObjectIDFromHex(receiverID)
	if err != nil {
		return nil, errors.New("invalid receiver ID")
	}

	// Check friendship status with cache
	cacheKey := "friends:" + msg.SenderID.Hex() + ":" + receiverID
	areFriends, err := s.redisClient.Get(ctx, cacheKey).Result()
	if err != nil || areFriends != "true" {
		// Fallback to database check
		areFriendsDB, err := s.friendshipRepo.AreFriends(ctx, msg.SenderID, rID)
		if err != nil {
			return nil, err
		}
		if !areFriendsDB {
			return nil, errors.New("can only message friends")
		}
		// Update cache
		s.redisClient.Set(ctx, cacheKey, "true", 1*time.Hour)
	}

	msg.ReceiverID = rID

	// Get sender info from cache or DB
	senderName, err := s.redisClient.Get(ctx, "user:"+msg.SenderID.Hex()+":name").Result()
	if err != nil {
		// In a real app, you'd fetch from user repo
		senderName = "Unknown"
	}
	msg.SenderName = senderName

	// Save to database
	createdMsg, err := s.messageRepo.CreateMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	// Publish to Kafka
	if err := s.producer.ProduceMessage(ctx, *createdMsg); err != nil {
		// Log error but don't fail the operation
		log.Printf("Failed to produce message to Kafka: %v", err)
	}

	// Update last message cache
	s.redisClient.Set(ctx, 
		"last_msg:"+msg.SenderID.Hex()+":"+receiverID, 
		createdMsg.ID.Hex(), 
		24*time.Hour,
	)

	return createdMsg, nil
}

func (s *MessageService) MarkMessagesAsSeen(ctx context.Context, userID primitive.ObjectID, messageIDs []primitive.ObjectID) error {
	if len(messageIDs) == 0 {
		return nil
	}

	// Update in database
	err := s.messageRepo.MarkMessagesAsSeen(ctx, userID, messageIDs)
	if err != nil {
		return err
	}

	// Update unread count in Redis
	for _, msgID := range messageIDs {
		s.redisClient.Decr(ctx, "unread:"+userID.Hex()+":"+msgID.Hex())
	}

	return nil
}

func (s *MessageService) GetUnreadCount(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	// Try Redis first
	count, err := s.redisClient.Get(ctx, "unread:"+userID.Hex()).Int64()
	if err == nil {
		return count, nil
	}

	// Fallback to database
	return s.messageRepo.GetUnreadCount(ctx, userID)
}

func (s *MessageService) GetConversationMessageTotalCount(
    ctx context.Context,
    query models.MessageQuery,
) (int64, error) {
    // Validate required parameters
	var isGroup bool = false
    if query.ConversationID == "" {
        return 0, errors.New("conversation ID is required")
    }

	if query.GroupID != "" {
		isGroup = true
	}

    // Generate cache key
    cacheKey := fmt.Sprintf("msg_count:%s:%t", query.ConversationID, isGroup)
    
    // Try Redis first
    count, err := s.redisClient.Get(ctx, cacheKey).Int64()
    if err == nil {
        return count, nil
    }

    // Convert string ID to ObjectID
    objID, err := primitive.ObjectIDFromHex(query.ConversationID)
    if err != nil {
        return 0, errors.New("invalid conversation ID format")
    }

    // Get count from repository
    count, err = s.messageRepo.GetConversationMessageCount(ctx, objID, isGroup)
    if err != nil {
        return 0, fmt.Errorf("failed to count messages: %w", err)
    }

    // Update cache with 3 second expiration
    s.redisClient.Set(ctx, cacheKey, count, 3*time.Second)
    return count, nil
}

func (s *MessageService) GetAllMessages(ctx context.Context, query models.MessageQuery) ([]models.Message, error) {
    return s.messageRepo.GetMessages(ctx, query)
}

// DeleteMessage handles message deletion with these features:
// 1. Validates message ownership
// 2. Performs soft-delete in database
// 3. Cleans up media files asynchronously
// 4. Publishes deletion event to Kafka
// 5. Updates relevant caches
func (s *MessageService) DeleteMessage(
    ctx context.Context,
    messageIDStr string,
    requesterID primitive.ObjectID,
) (*models.Message, error) {
    messageID, err := primitive.ObjectIDFromHex(messageIDStr)
    if err != nil {
        return nil, errors.New("invalid message ID format")
    }

    // TODO: Media deletion function using storage service
    mediaDeleter := func(ctx context.Context, urls []string) error {
        if len(urls) == 0 {
            return nil
        }
        
        // TODO: In production, will use actual media service:
        // return s.mediaService.DeleteFiles(ctx, urls)
        
        // Mock implementation:
        log.Printf("Deleting media files: %v", urls)
        return nil
    }

    deletedMsg, err := s.messageRepo.DeleteMessage(ctx, messageID, requesterID, mediaDeleter)
    if err != nil {
        return nil, err
    }

    // Publish deletion event to Kafka
    if err := s.producer.ProduceMessage(ctx, models.Message{
        ID:          deletedMsg.ID,
        SenderID:    deletedMsg.SenderID,
        ReceiverID:  deletedMsg.ReceiverID,
        GroupID:     deletedMsg.GroupID,
        ContentType: models.ContentTypeDeleted,
        DeletedAt:   deletedMsg.DeletedAt,
    }); err != nil {
        log.Printf("Failed to publish deletion event: %v", err)
    }

    if !deletedMsg.GroupID.IsZero() {
        cacheKey := "group_last_msg:" + deletedMsg.GroupID.Hex()
        s.redisClient.Del(ctx, cacheKey)
    } else {
        cacheKey := fmt.Sprintf("last_msg:%s:%s", 
            deletedMsg.SenderID.Hex(),
            deletedMsg.ReceiverID.Hex())
        s.redisClient.Del(ctx, cacheKey)
    }

    return deletedMsg, nil
}