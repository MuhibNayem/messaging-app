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

func (s *StoryService) GetStoriesFeed(ctx context.Context, userID primitive.ObjectID, limit, offset int) ([]models.Story, error) {
	// Get friends
	friends, err := s.friendshipRepo.GetFriends(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Collect user IDs (friends + self)
	userIDs := make([]primitive.ObjectID, len(friends)+1)
	userIDs[0] = userID
	friendMap := make(map[string]bool)

	for i, friend := range friends {
		userIDs[i+1] = friend.ID
		friendMap[friend.ID.Hex()] = true
	}

	// 1. Get Paginated Authors (User IDs who have active stories)
	authorIDs, err := s.storyRepo.GetActiveStoryAuthors(ctx, userIDs, limit, offset)
	if err != nil {
		return nil, err
	}

	if len(authorIDs) == 0 {
		return []models.Story{}, nil
	}

	// 2. Fetch Stories for these authors
	stories, err := s.storyRepo.GetStoriesForUsers(ctx, authorIDs)
	if err != nil {
		return nil, err
	}

	// 3. Filter based on privacy
	var visibleStories []models.Story
	for _, story := range stories {
		// Own stories always visible
		if story.UserID == userID {
			visibleStories = append(visibleStories, story)
			continue
		}

		// Privacy Check
		switch story.Privacy {
		case models.PrivacySettingPublic:
			visibleStories = append(visibleStories, story)
		case models.PrivacySettingFriends:
			if friendMap[story.UserID.Hex()] {
				visibleStories = append(visibleStories, story)
			}
		case models.PrivacySettingOnlyMe:
			continue
		case models.PrivacySettingCustom:
			for _, allowedID := range story.AllowedViewers {
				if allowedID == userID {
					visibleStories = append(visibleStories, story)
					break
				}
			}
		case models.PrivacySettingFriendsExcept:
			blocked := false
			for _, blockedID := range story.BlockedViewers {
				if blockedID == userID {
					blocked = true
					break
				}
			}
			if !blocked {
				visibleStories = append(visibleStories, story)
			}
		default:
			// Default to friends
			if friendMap[story.UserID.Hex()] {
				visibleStories = append(visibleStories, story)
			}
		}
	}

	return visibleStories, nil
}

func (s *StoryService) GetUserStories(ctx context.Context, userID primitive.ObjectID) ([]models.Story, error) {
	return s.storyRepo.GetUserStories(ctx, userID)
}

func (s *StoryService) DeleteStory(ctx context.Context, storyID primitive.ObjectID, userID primitive.ObjectID) error {
	return s.storyRepo.DeleteStory(ctx, storyID, userID)
}

func (s *StoryService) RecordView(ctx context.Context, storyID primitive.ObjectID, userID primitive.ObjectID) error {
	return s.storyRepo.AddViewer(ctx, storyID, userID)
}

func (s *StoryService) ReactToStory(ctx context.Context, storyID primitive.ObjectID, userID primitive.ObjectID, reactionType string) error {
	reaction := models.StoryReaction{
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
