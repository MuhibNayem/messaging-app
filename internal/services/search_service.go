package services

import (
	"context"
	"log"

	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SearchService struct {
	userRepo *repositories.UserRepository
	feedRepo *repositories.FeedRepository
}

func NewSearchService(userRepo *repositories.UserRepository, feedRepo *repositories.FeedRepository) *SearchService {
	return &SearchService{
		userRepo: userRepo,
		feedRepo: feedRepo,
	}
}

type SearchResult struct {
	Users []models.SafeUserResponse `json:"users"`
	Posts []models.Post             `json:"posts"`
	Total int64                     `json:"total"`
}

func (s *SearchService) Search(ctx context.Context, query string, page, limit int64) (*SearchResult, error) {
	if query == "" {
		return &SearchResult{}, nil
	}

	var searchResult SearchResult

	// Search Users
	userFilter := bson.M{"$text": bson.M{"$search": query}}
	userFindOptions := options.Find().SetSkip((page - 1) * limit).SetLimit(limit)
	users, err := s.userRepo.FindUsers(ctx, userFilter, userFindOptions)
	if err != nil && err != mongo.ErrNoDocuments {
		log.Printf("Error searching users: %v", err)
	} else if err == nil {
		safeUsers := make([]models.SafeUserResponse, len(users))
		for i, user := range users {
			safeUsers[i] = user.ToSafeResponse()
		}
		searchResult.Users = safeUsers
		// For total count, we need to count without skip/limit
		totalUsers, err := s.userRepo.CountUsers(ctx, userFilter)
		if err != nil {
			log.Printf("Error counting users: %v", err)
		} else {
			searchResult.Total += totalUsers
		}
	}

	// Search Posts
	postFilter := bson.M{"$text": bson.M{"$search": query}}
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
