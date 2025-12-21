package services

import (
	"context"
	"log"

	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SearchService struct {
	userRepo       *repositories.UserRepository
	feedRepo       *repositories.FeedRepository
	friendshipRepo *repositories.FriendshipRepository
}

func NewSearchService(userRepo *repositories.UserRepository, feedRepo *repositories.FeedRepository, friendshipRepo *repositories.FriendshipRepository) *SearchService {
	return &SearchService{
		userRepo:       userRepo,
		feedRepo:       feedRepo,
		friendshipRepo: friendshipRepo,
	}
}

// UserWithFriendshipStatus extends SafeUserResponse with friendship info
type UserWithFriendshipStatus struct {
	models.SafeUserResponse
	FriendshipStatus *FriendshipStatusResponse `json:"friendship_status,omitempty"`
}

type SearchResult struct {
	Users []UserWithFriendshipStatus `json:"users"`
	Posts []models.Post              `json:"posts"`
	Total int64                      `json:"total"`
}

func (s *SearchService) Search(ctx context.Context, query string, page, limit int64, currentUserID *primitive.ObjectID) (*SearchResult, error) {
	if query == "" {
		return &SearchResult{}, nil
	}

	var searchResult SearchResult
	regexQuery := bson.M{"$regex": query, "$options": "i"} // i for case-insensitive

	// Search Users
	userFilter := bson.M{"$or": []bson.M{
		{"username": regexQuery},
		{"email": regexQuery},
		{"full_name": regexQuery},
	}}
	userFindOptions := options.Find().SetSkip((page - 1) * limit).SetLimit(limit)
	users, err := s.userRepo.FindUsers(ctx, userFilter, userFindOptions)
	if err != nil && err != mongo.ErrNoDocuments {
		log.Printf("Error searching users: %v", err)
	} else if err == nil {
		usersWithStatus := make([]UserWithFriendshipStatus, len(users))
		for i, user := range users {
			usersWithStatus[i] = UserWithFriendshipStatus{
				SafeUserResponse: user.ToSafeResponse(),
			}

			// Add friendship status if currentUserID is provided and not self
			if currentUserID != nil && user.ID != *currentUserID {
				status, err := s.getFriendshipStatus(ctx, *currentUserID, user.ID)
				if err == nil {
					usersWithStatus[i].FriendshipStatus = status
				}
			}
		}
		searchResult.Users = usersWithStatus
		// For total count, we need to count without skip/limit
		totalUsers, err := s.userRepo.CountUsers(ctx, userFilter)
		if err != nil {
			log.Printf("Error counting users: %v", err)
		} else {
			searchResult.Total += totalUsers
		}
	}

	// Search Posts (only public posts)
	postFilter := bson.M{
		"privacy": models.PrivacySettingPublic,
		"$or": []bson.M{
			{"content": regexQuery},
			{"hashtags": regexQuery},
		},
	}
	postFindOptions := options.Find().SetSkip((page - 1) * limit).SetLimit(limit)
	posts, err := s.feedRepo.ListPosts(ctx, postFilter, postFindOptions)
	if err != nil && err != mongo.ErrNoDocuments {
		log.Printf("Error searching posts: %v", err)
	} else if err == nil {
		searchResult.Posts = posts
		// For total count, we need to count without skip/limit
		totalPosts, err := s.feedRepo.CountPosts(ctx, postFilter)
		if err != nil {
			log.Printf("Error counting posts: %v", err)
		} else {
			searchResult.Total += totalPosts
		}
	}

	return &searchResult, nil
}

// getFriendshipStatus returns the friendship status between two users
func (s *SearchService) getFriendshipStatus(ctx context.Context, viewerID, otherUserID primitive.ObjectID) (*FriendshipStatusResponse, error) {
	response := &FriendshipStatusResponse{}

	// Check if they are friends
	areFriends, err := s.friendshipRepo.AreFriends(ctx, viewerID, otherUserID)
	if err != nil {
		return nil, err
	}
	response.AreFriends = areFriends

	if areFriends {
		return response, nil
	}

	// Check for pending requests (viewer sent)
	_, err = s.friendshipRepo.GetPendingRequest(ctx, viewerID, otherUserID)
	response.RequestSent = err == nil

	// Check for pending requests (viewer received)
	_, err = s.friendshipRepo.GetPendingRequest(ctx, otherUserID, viewerID)
	response.RequestReceived = err == nil

	// Check block status
	isBlockedByViewer, _ := s.friendshipRepo.IsBlockedBy(ctx, otherUserID, viewerID)
	hasBlockedViewer, _ := s.friendshipRepo.IsBlockedBy(ctx, viewerID, otherUserID)
	response.IsBlockedByViewer = isBlockedByViewer
	response.HasBlockedViewer = hasBlockedViewer

	return response, nil
}
