package services

import (
	"context"
	"log"
	"messaging-app/internal/repositories"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CleanupService struct {
	storyRepo      *repositories.StoryRepository
	storageService *StorageService
}

func NewCleanupService(storyRepo *repositories.StoryRepository, storageService *StorageService) *CleanupService {
	return &CleanupService{
		storyRepo:      storyRepo,
		storageService: storageService,
	}
}

// StartCleanupWorker starts a background ticker to clean up expired stories
func (s *CleanupService) StartCleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // Run every hour
	defer ticker.Stop()

	// Run once immediately on startup
	go s.cleanupExpiredStories(ctx)

	for {
		select {
		case <-ticker.C:
			go s.cleanupExpiredStories(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *CleanupService) cleanupExpiredStories(ctx context.Context) {
	// Create a detached context for cleanup operations so they don't get cancelled immediately if main ctx cancels
	// However, usually we pass the main application context which signals server shutdown.
	// We want to handle graceful shutdown, but for background task simplicity we use the passed context.

	log.Println("Running expired story cleanup...")

	// 1. Get expired stories
	stories, err := s.storyRepo.GetExpiredStories(ctx)
	if err != nil {
		log.Printf("Failed to fetch expired stories: %v", err)
		return
	}

	if len(stories) == 0 {
		return
	}

	log.Printf("Found %d expired stories to clean up", len(stories))

	var idsToDelete []primitive.ObjectID

	// 2. Delete files and collect IDs
	for _, story := range stories {
		if story.MediaURL != "" {
			err := s.storageService.DeleteFile(ctx, story.MediaURL)
			if err != nil {
				log.Printf("Failed to delete file for story %s: %v", story.ID.Hex(), err)
				// Continue to cleanup DB anyway
			}
		}
		idsToDelete = append(idsToDelete, story.ID)
	}

	// 3. Delete records from DB
	if len(idsToDelete) > 0 {
		err := s.storyRepo.DeleteStories(ctx, idsToDelete)
		if err != nil {
			log.Printf("Failed to delete expired story records: %v", err)
		} else {
			log.Printf("Successfully cleaned up %d expired stories", len(idsToDelete))
		}
	}
}
