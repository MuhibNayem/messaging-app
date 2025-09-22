package services

import (
	"context"
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ConversationService struct {
	conversationRepo *repositories.ConversationRepository
}

func NewConversationService(cr *repositories.ConversationRepository) *ConversationService {
	return &ConversationService{
		conversationRepo: cr,
	}
}

func (s *ConversationService) GetConversationSummaries(ctx context.Context, userID primitive.ObjectID) ([]models.ConversationSummary, error) {
	log.Printf("Service: GetConversationSummaries for user %s", userID.Hex())
	summaries, err := s.conversationRepo.GetConversationSummaries(ctx, userID)
	if err != nil {
		log.Printf("Service: Error from conversation repository for user %s: %v", userID.Hex(), err)
		return nil, err
	}
	log.Printf("Service: Retrieved %d conversation summaries for user %s", len(summaries), userID.Hex())
	return summaries, nil
}
