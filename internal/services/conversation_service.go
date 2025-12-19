package services

import (
	"context"
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ConversationService struct {
	conversationRepo     *repositories.ConversationRepository
	messageCassandraRepo *repositories.MessageCassandraRepository
	userRepo             *repositories.UserRepository
	groupRepo            *repositories.GroupRepository
}

func NewConversationService(cr *repositories.ConversationRepository, mcr *repositories.MessageCassandraRepository, ur *repositories.UserRepository, gr *repositories.GroupRepository) *ConversationService {
	return &ConversationService{
		conversationRepo:     cr,
		messageCassandraRepo: mcr,
		userRepo:             ur,
		groupRepo:            gr,
	}
}

func (s *ConversationService) GetConversationSummaries(ctx context.Context, userID primitive.ObjectID) ([]models.ConversationSummary, error) {
	log.Printf("Service: GetConversationSummaries for user %s (Cassandra)", userID.Hex())
	// Use Cassandra for scalable inbox
	summaries, err := s.messageCassandraRepo.GetInbox(ctx, userID, false) // isMarketplace = false
	if err != nil {
		log.Printf("Service: Error from Cassandra inbox for user %s: %v", userID.Hex(), err)
		return nil, err
	}

	// Batch-fetch avatars for scalability (O(2) queries instead of O(N))
	// Step 1: Collect unique user IDs and group IDs
	userIDs := make(map[primitive.ObjectID]bool)
	groupIDs := make(map[primitive.ObjectID]bool)

	for _, conv := range summaries {
		if conv.IsGroup {
			groupIDStr := strings.TrimPrefix(conv.ID, "group-")
			if groupID, err := primitive.ObjectIDFromHex(groupIDStr); err == nil {
				groupIDs[groupID] = true
			}
		} else {
			userIDStr := strings.TrimPrefix(conv.ID, "user-")
			if partnerID, err := primitive.ObjectIDFromHex(userIDStr); err == nil {
				userIDs[partnerID] = true
			}
		}
	}

	// Step 2: Batch fetch users (1 query for all users)
	userMap := make(map[string]*models.User)
	if len(userIDs) > 0 {
		userIDList := make([]primitive.ObjectID, 0, len(userIDs))
		for uid := range userIDs {
			userIDList = append(userIDList, uid)
		}
		users, err := s.userRepo.FindUsersByIDs(ctx, userIDList)
		if err == nil {
			for i := range users {
				userMap[users[i].ID.Hex()] = &users[i]
			}
		}
	}

	// Step 3: Batch fetch groups (1 query for all groups)
	groupMap := make(map[string]*models.Group)
	if len(groupIDs) > 0 {
		groupIDList := make([]primitive.ObjectID, 0, len(groupIDs))
		for gid := range groupIDs {
			groupIDList = append(groupIDList, gid)
		}
		groups, err := s.groupRepo.GetGroupsByIDs(ctx, groupIDList)
		if err == nil {
			for _, group := range groups {
				groupMap[group.ID.Hex()] = group
			}
		}
	}

	// Step 4: Enrich summaries with avatar data (in-memory join)
	for i := range summaries {
		conv := &summaries[i]

		if conv.IsGroup {
			groupIDStr := strings.TrimPrefix(conv.ID, "group-")
			if group, ok := groupMap[groupIDStr]; ok {
				conv.Avatar = group.Avatar
				conv.Name = group.Name
			}
		} else {
			userIDStr := strings.TrimPrefix(conv.ID, "user-")
			if user, ok := userMap[userIDStr]; ok {
				conv.Avatar = user.Avatar
			}
		}
	}

	log.Printf("Service: Retrieved %d conversation summaries for user %s from Cassandra", len(summaries), userID.Hex())
	return summaries, nil
}
