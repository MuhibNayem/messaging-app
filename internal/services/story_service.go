package services

import (
	"context"
	"errors"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type StoryService struct {
	storyRepo      *repositories.StoryRepository
	userRepo       *repositories.UserRepository
	friendshipRepo *repositories.FriendshipRepository
}

func NewStoryService(storyRepo *repositories.StoryRepository, userRepo *repositories.UserRepository, friendshipRepo *repositories.FriendshipRepository) *StoryService {
	return &StoryService{
		storyRepo:      storyRepo,
		userRepo:       userRepo,
		friendshipRepo: friendshipRepo,
	}
}

func (s *StoryService) CreateStory(ctx context.Context, userID primitive.ObjectID, req *models.CreateStoryRequest) (*models.Story, error) {
	// Fetch user details for author info
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	privacy := req.Privacy
	if privacy == "" {
		privacy = models.PrivacySettingFriends
	}

	story := &models.Story{
		UserID:         userID,
		MediaURL:       req.MediaURL,
		MediaType:      req.MediaType,
		Privacy:        privacy,
		AllowedViewers: req.AllowedViewers,
		BlockedViewers: req.BlockedViewers,
		Author: models.PostAuthor{
			ID:       user.ID.Hex(),
			Username: user.Username,
			Avatar:   user.Avatar,
			FullName: user.FullName,
		},
	}

	return s.storyRepo.CreateStory(ctx, story)
}

// GetStoriesFeed returns the stories feed for the user with DB-level privacy filtering
func (s *StoryService) GetStoriesFeed(ctx context.Context, userID primitive.ObjectID, limit, offset int) ([]models.Story, error) {
	// Get friends
	friends, err := s.friendshipRepo.GetFriends(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Collect user IDs (friends + self)
	userIDs := make([]primitive.ObjectID, len(friends)+1)
	userIDs[0] = userID

	for i, friend := range friends {
		userIDs[i+1] = friend.ID
	}

	// 1. Get Paginated Authors (User IDs who have active stories AND visible to userID)
	authorIDs, err := s.storyRepo.GetActiveStoryAuthors(ctx, userID, userIDs, limit, offset)
	if err != nil {
		return nil, err
	}

	if len(authorIDs) == 0 {
		return []models.Story{}, nil
	}

	// 2. Fetch Stories for these authors (filtered by privacy at DB level)
	stories, err := s.storyRepo.GetStoriesForUsers(ctx, userID, authorIDs)
	if err != nil {
		return nil, err
	}

	// No further in-memory filtering needed!

	return stories, nil
}

func (s *StoryService) GetUserStories(ctx context.Context, userID primitive.ObjectID) ([]models.Story, error) {
	return s.storyRepo.GetUserStories(ctx, userID)
}

func (s *StoryService) DeleteStory(ctx context.Context, storyID primitive.ObjectID, userID primitive.ObjectID) error {
	return s.storyRepo.DeleteStory(ctx, storyID, userID)
}

func (s *StoryService) RecordView(ctx context.Context, storyID primitive.ObjectID, userID primitive.ObjectID) error {
	// Fetch story to check author
	story, err := s.storyRepo.GetStoryByID(ctx, storyID)
	if err != nil {
		return err
	}

	// Don't count self-views
	if story.UserID == userID {
		return nil
	}

	return s.storyRepo.AddViewer(ctx, storyID, userID)
}

func (s *StoryService) ReactToStory(ctx context.Context, storyID primitive.ObjectID, userID primitive.ObjectID, reactionType string) error {
	reaction := models.StoryReaction{
		StoryID:   storyID,
		UserID:    userID,
		Type:      reactionType,
		CreatedAt: time.Now(),
	}
	return s.storyRepo.AddReaction(ctx, storyID, reaction)
}

func (s *StoryService) GetStoryViewers(ctx context.Context, storyID primitive.ObjectID, userID primitive.ObjectID) ([]models.StoryViewerResponse, error) {
	// 1. Fetch story to check ownership
	story, err := s.storyRepo.GetStoryByID(ctx, storyID)
	if err != nil {
		return nil, err
	}
	if story.UserID != userID {
		return nil, errors.New("unauthorized: only author can view viewers")
	}

	// 2. Fetch Viewers from Repo (which joins Views + Reactions)
	return s.storyRepo.GetStoryViewersWithReactions(ctx, storyID)
}
