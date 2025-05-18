package repositories

import (
	"context"
	"errors"
	"fmt"
	"log"
	"messaging-app/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MessageRepository struct {
	db         *mongo.Database
	collection *mongo.Collection
}

func NewMessageRepository(db *mongo.Database) *MessageRepository {
	collection := db.Collection("messages")
	
	// Compound indexes for faster queries
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "sender_id", Value: 1},
				{Key: "receiver_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "group_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "content_type", Value: 1}},
		},
		// TTL index for auto-deleting messages after 1 year
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(365 * 24 * 60 * 60),
		},
	}

	_, err := collection.Indexes().CreateMany(context.Background(), indexes)
	if err != nil {
		panic("Failed to create message indexes: " + err.Error())
	}

	return &MessageRepository{
		db:         db,
		collection: collection,
	}
}

func (r *MessageRepository) GetMessages(ctx context.Context, query models.MessageQuery) ([]models.Message, error) {
	filter := bson.M{}

	if query.GroupID != "" {
		groupID, err := primitive.ObjectIDFromHex(query.GroupID)
		if err != nil {
			return nil, errors.New("invalid group ID")
		}
		filter["group_id"] = groupID
	} else if query.ReceiverID != "" {
		receiverID, err := primitive.ObjectIDFromHex(query.ReceiverID)
		if err != nil {
			return nil, errors.New("invalid receiver ID")
		}
		filter["$or"] = []bson.M{
			{"sender_id": query.SenderID, "receiver_id": receiverID},
			{"sender_id": receiverID, "receiver_id": query.SenderID},
		}
	} else {
		return nil, errors.New("either group_id or receiver_id must be provided")
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((query.Page - 1) * query.Limit)).
		SetLimit(int64(query.Limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *MessageRepository) CreateMessage(ctx context.Context, msg *models.Message) (*models.Message, error) {
	msg.CreatedAt = time.Now()
	msg.UpdatedAt = time.Now()

	res, err := r.collection.InsertOne(ctx, msg)
	if err != nil {
		return nil, err
	}

	msg.ID = res.InsertedID.(primitive.ObjectID)
	return msg, nil
}

func (r *MessageRepository) MarkMessagesAsSeen(ctx context.Context, userID primitive.ObjectID, messageIDs []primitive.ObjectID) error {
	_, err := r.collection.UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": messageIDs}},
		bson.M{
			"$addToSet": bson.M{"seen_by": userID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

func (r *MessageRepository) GetUnreadCount(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{
		"receiver_id": userID,
		"seen_by":     bson.M{"$ne": userID},
	})
}

func (r *MessageRepository) GetConversationMessageCount(
    ctx context.Context,
    conversationID primitive.ObjectID,
    isGroup bool,
) (int64, error) {
    filter := bson.M{
    }

    if isGroup {
        filter["group_id"] = conversationID
    } else {
        filter["$or"] = []bson.M{
            {"sender_id": conversationID},
            {"receiver_id": conversationID},
        }
    }

    count, err := r.collection.CountDocuments(ctx, filter)
    if err != nil {
        return 0, fmt.Errorf("failed to count messages: %w", err)
    }

    return count, nil
}

func (r *MessageRepository) DeleteMessage(
    ctx context.Context,
    messageID primitive.ObjectID,
    requesterID primitive.ObjectID,
    mediaDeleter func(ctx context.Context, urls []string) error,
) (*models.Message, error) {
	log.Printf("Deleting message with ID: %s by user: %s", messageID.Hex(), requesterID.Hex())
    var deletedMessage models.Message
    err := r.collection.FindOneAndUpdate(
        ctx,
        bson.M{
            "_id":       messageID,
            "sender_id": requesterID, 
        },
        bson.M{
            "$set": bson.M{
                "deleted_at":     time.Now(),
                "is_deleted":     true,
                "original_content": "$content", 
                "content":        "[deleted]",
                "media_urls":     []string{},
                "content_type":   models.ContentTypeDeleted,
            },
        },
        options.FindOneAndUpdate().
            SetReturnDocument(options.After).
            SetProjection(bson.M{
                "original_content": 0, 
            }),
    ).Decode(&deletedMessage)

    if err != nil {
        if err == mongo.ErrNoDocuments {
            return nil, errors.New("message not found or not owned by user")
        }
        return nil, err
    }

    // Async media cleanup
    if len(deletedMessage.MediaURLs) > 0 && mediaDeleter != nil {
        go func() {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()
            
            if err := mediaDeleter(ctx, deletedMessage.MediaURLs); err != nil {
                log.Printf("Failed to cleanup media for message %s: %v", messageID.Hex(), err)
            }
        }()
    }

    return &deletedMessage, nil
}