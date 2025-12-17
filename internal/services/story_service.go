package services

import (
	"context"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

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

func (s *StoryService) GetStoriesFeed(ctx context.Context, userID primitive.ObjectID) ([]models.Story, error) {
	// Get friends
	friends, err := s.friendshipRepo.GetFriends(ctx, userID)
	if err != nil {
		return nil, err
	}

	// For TRUE public stories, we might want to fetch stories from non-friends too?
	// But standard feed is usually followed/friends.
	// However, user said "Public must be public".
	// If this means "Anyone can see my public story if they look for it", that is satisfied by access control.
	// If it means "My feed should include public stories from strangers", that's a "Global Feed".
	// Assuming standard "Stories Feed" = Friends + Self, but respecting privacy settings.

	// Collect user IDs (friends + self)
	userIDs := make([]primitive.ObjectID, len(friends)+1)
	userIDs[0] = userID
	friendMap := make(map[string]bool)

	for i, friend := range friends {
		userIDs[i+1] = friend.ID
		friendMap[friend.ID.Hex()] = true
	}

	stories, err := s.storyRepo.GetActiveStories(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	// Filter based on privacy
	var visibleStories []models.Story
	for _, story := range stories {
		// 1. Own stories always visible
		if story.UserID == userID {
			visibleStories = append(visibleStories, story)
			continue
		}

		// 2. Privacy Check
		switch story.Privacy {
		case models.PrivacySettingPublic:
			visibleStories = append(visibleStories, story)
		case models.PrivacySettingFriends:
			// Viewer must be friend (already ensured by GetActiveStories taking friends list,
			// BUT if we expand later to non-friends, we need this check.
			// Currently, `stories` only contains friends+self, so this is implicit, but let's be safe.)
			if friendMap[story.UserID.Hex()] {
				visibleStories = append(visibleStories, story)
			}
		case models.PrivacySettingOnlyMe:
			// Only author sees (handled by `story.UserID == userID` above)
			continue
		case models.PrivacySettingCustom:
			// Check if viewer ID is in AllowedViewers
			for _, allowedID := range story.AllowedViewers {
				if allowedID == userID {
					visibleStories = append(visibleStories, story)
					break
				}
			}
		case models.PrivacySettingFriendsExcept:
			// Check if viewer ID is NOT in BlockedViewers
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
			// Default to friends? Or hide? Let's say Friends default.
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
