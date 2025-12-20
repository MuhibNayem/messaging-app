package repositories

import (
	"context"
	"errors"
	"time"

	"messaging-app/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EventPostRepository struct {
	collection *mongo.Collection
}

func NewEventPostRepository(db *mongo.Database) *EventPostRepository {
	collection := db.Collection("event_posts")

	// Create indexes for optimized post queries
	_, err := collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		// Event ID + created_at for paginated post listing
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index(),
		},
		// Author ID for "my posts" functionality
		{
			Keys:    bson.D{{Key: "author_id", Value: 1}},
			Options: options.Index(),
		},
	})
	if err != nil {
		// Log but don't panic - indexes may already exist
	}

	return &EventPostRepository{
		collection: collection,
	}
}

// Create creates a new discussion post for an event
func (r *EventPostRepository) Create(ctx context.Context, post *models.EventPost) error {
	post.CreatedAt = time.Now()
	post.UpdatedAt = time.Now()
	if post.Reactions == nil {
		post.Reactions = []models.EventPostReaction{}
	}
	if post.MediaURLs == nil {
		post.MediaURLs = []string{}
	}

	result, err := r.collection.InsertOne(ctx, post)
	if err != nil {
		return err
	}

	post.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// GetByID retrieves a post by ID
func (r *EventPostRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.EventPost, error) {
	var post models.EventPost
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&post)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("post not found")
		}
		return nil, err
	}
	return &post, nil
}

// GetByEventID retrieves all posts for an event with pagination
func (r *EventPostRepository) GetByEventID(ctx context.Context, eventID primitive.ObjectID, limit, page int64) ([]models.EventPost, int64, error) {
	filter := bson.M{"event_id": eventID}

	skip := (page - 1) * limit
	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.M{"created_at": -1}) // Newest first

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var posts []models.EventPost
	if err = cursor.All(ctx, &posts); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

// Update updates a post
func (r *EventPostRepository) Update(ctx context.Context, post *models.EventPost) error {
	post.UpdatedAt = time.Now()
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": post.ID}, post)
	return err
}

// Delete removes a post
func (r *EventPostRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// DeleteByEventID removes all posts for an event (cleanup when event deleted)
func (r *EventPostRepository) DeleteByEventID(ctx context.Context, eventID primitive.ObjectID) error {
	_, err := r.collection.DeleteMany(ctx, bson.M{"event_id": eventID})
	return err
}

// AddReaction adds a reaction to a post
func (r *EventPostRepository) AddReaction(ctx context.Context, postID primitive.ObjectID, reaction models.EventPostReaction) error {
	// First, remove existing reaction from same user (if any)
	_, _ = r.collection.UpdateOne(ctx, bson.M{"_id": postID}, bson.M{
		"$pull": bson.M{"reactions": bson.M{"user_id": reaction.UserID}},
	})

	// Then add the new reaction
	update := bson.M{
		"$push": bson.M{"reactions": reaction},
		"$set":  bson.M{"updated_at": time.Now()},
	}

	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": postID}, update)
	return err
}

// RemoveReaction removes a user's reaction from a post
func (r *EventPostRepository) RemoveReaction(ctx context.Context, postID, userID primitive.ObjectID) error {
	update := bson.M{
		"$pull": bson.M{"reactions": bson.M{"user_id": userID}},
		"$set":  bson.M{"updated_at": time.Now()},
	}

	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": postID}, update)
	return err
}

// GetPostCount returns the total number of posts for an event
func (r *EventPostRepository) GetPostCount(ctx context.Context, eventID primitive.ObjectID) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"event_id": eventID})
}
