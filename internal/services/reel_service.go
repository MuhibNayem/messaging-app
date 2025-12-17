package services

import (
	"context"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ReelService struct {
	reelRepo *repositories.ReelRepository
	userRepo *repositories.UserRepository
}

func NewReelService(reelRepo *repositories.ReelRepository, userRepo *repositories.UserRepository) *ReelService {
	return &ReelService{
		reelRepo: reelRepo,
		userRepo: userRepo,
	}
}

func (s *ReelService) CreateReel(ctx context.Context, userID primitive.ObjectID, req *models.CreateReelRequest) (*models.Reel, error) {
	// Fetch user for author info
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	reel := &models.Reel{
		UserID:       userID,
		VideoURL:     req.VideoURL,
		ThumbnailURL: req.ThumbnailURL,
		Caption:      req.Caption,
		Duration:     req.Duration,
		Author: models.PostAuthor{
			ID:       user.ID.Hex(),
			Username: user.Username,
			Avatar:   user.Avatar,
			FullName: user.FullName,
		},
	}

	return s.reelRepo.CreateReel(ctx, reel)
}

func (s *ReelService) GetReelsFeed(ctx context.Context, limit, offset int64) ([]models.Reel, error) {
	// For now, just return latest reels.
	// In future, this would be an algorithmic feed.
	return s.reelRepo.ListReels(ctx, limit, offset)
}

func (s *ReelService) GetUserReels(ctx context.Context, userID primitive.ObjectID) ([]models.Reel, error) {
	return s.reelRepo.GetUserReels(ctx, userID)
}

func (s *ReelService) GetReel(ctx context.Context, reelID primitive.ObjectID) (*models.Reel, error) {
	return s.reelRepo.GetReelByID(ctx, reelID)
}

func (s *ReelService) DeleteReel(ctx context.Context, reelID primitive.ObjectID, userID primitive.ObjectID) error {
	return s.reelRepo.DeleteReel(ctx, reelID, userID)
}

func (s *ReelService) IncrementViews(ctx context.Context, reelID primitive.ObjectID) error {
	return s.reelRepo.IncrementViews(ctx, reelID)
}
