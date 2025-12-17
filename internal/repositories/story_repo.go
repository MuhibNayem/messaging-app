package repositories

import (
	"context"
	"time"

	"messaging-app/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type StoryRepository struct {
	collection *mongo.Collection
}

func NewStoryRepository(db *mongo.Database) *StoryRepository {
	// Create indexes
	_, err := db.Collection("stories").Indexes().CreateMany(
		context.Background(),
		[]mongo.IndexModel{
			{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index()},
			{Keys: bson.D{{Key: "created_at", Value: -1}}, Options: options.Index()},
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index()}, // For cleaner/Active queries
		},
	)
	if err != nil {
		panic("Failed to create story indexes: " + err.Error())
	}

	return &StoryRepository{
		collection: db.Collection("stories"),
	}
}

func (r *StoryRepository) CreateStory(ctx context.Context, story *models.Story) (*models.Story, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	story.CreatedAt = time.Now()
	if story.ExpiresAt.IsZero() {
		story.ExpiresAt = story.CreatedAt.Add(24 * time.Hour)
	}

	res, err := r.collection.InsertOne(ctx, story)
	if err != nil {
		return nil, err
	}
	story.ID = res.InsertedID.(primitive.ObjectID)
	return story, nil
}

func (r *StoryRepository) GetStoryByID(ctx context.Context, id primitive.ObjectID) (*models.Story, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var story models.Story
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&story)
	if err != nil {
		return nil, err
	}
	return &story, nil
}

func (r *StoryRepository) DeleteStory(ctx context.Context, id primitive.ObjectID, userID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	return err
}

func (r *StoryRepository) GetActiveStories(ctx context.Context, userIDs []primitive.ObjectID) ([]models.Story, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	now := time.Now()
	filter := bson.M{
		"user_id":    bson.M{"$in": userIDs},
		"expires_at": bson.M{"$gt": now},
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cur, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var stories []models.Story
	if err := cur.All(ctx, &stories); err != nil {
		return nil, err
	}
	return stories, nil
}

func (r *StoryRepository) GetUserStories(ctx context.Context, userID primitive.ObjectID) ([]models.Story, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	now := time.Now()

	// Get only active stories
	filter := bson.M{
		"user_id":    userID,
		"expires_at": bson.M{"$gt": now},
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cur, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var stories []models.Story
	if err := cur.All(ctx, &stories); err != nil {
		return nil, err
	}
	return stories, nil
}

// GetExpiredStories returns stories that have expired but are still in the database.
func (r *StoryRepository) GetExpiredStories(ctx context.Context) ([]models.Story, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	now := time.Now()
	filter := bson.M{"expires_at": bson.M{"$lte": now}}

	// Limit to batch size (e.g., 100) to avoid memory issues
	opts := options.Find().SetLimit(100)

	cur, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var stories []models.Story
	if err := cur.All(ctx, &stories); err != nil {
		return nil, err
	}
	return stories, nil
}

// DeleteStories bulk deletes stories by their IDs
func (r *StoryRepository) DeleteStories(ctx context.Context, ids []primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.collection.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	return err
}

func (r *StoryRepository) AddViewer(ctx context.Context, storyID primitive.ObjectID, viewerID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	filter := bson.M{"_id": storyID}
	update := bson.M{"$addToSet": bson.M{"viewers": viewerID}}
	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}

func (r *StoryRepository) AddReaction(ctx context.Context, storyID primitive.ObjectID, reaction models.StoryReaction) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	filter := bson.M{"_id": storyID}
	update := bson.M{"$push": bson.M{"reactions": reaction}}
	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}
