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

type ReelRepository struct {
	collection *mongo.Collection
}

func NewReelRepository(db *mongo.Database) *ReelRepository {
	_, err := db.Collection("reels").Indexes().CreateMany(
		context.Background(),
		[]mongo.IndexModel{
			{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index()},
			{Keys: bson.D{{Key: "created_at", Value: -1}}, Options: options.Index()},
			{Keys: bson.D{{Key: "views", Value: -1}}, Options: options.Index()}, // For trending
		},
	)
	if err != nil {
		panic("Failed to create reel indexes: " + err.Error())
	}

	return &ReelRepository{
		collection: db.Collection("reels"),
	}
}

func (r *ReelRepository) CreateReel(ctx context.Context, reel *models.Reel) (*models.Reel, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	reel.CreatedAt = time.Now()
	reel.UpdatedAt = time.Now()

	res, err := r.collection.InsertOne(ctx, reel)
	if err != nil {
		return nil, err
	}
	reel.ID = res.InsertedID.(primitive.ObjectID)
	return reel, nil
}

func (r *ReelRepository) GetReelByID(ctx context.Context, id primitive.ObjectID) (*models.Reel, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Using aggregation to join user info if not denormalized or just simpler find
	// Since we denormalized Author in model, simple find is OK for now, but aggregation allows better future proofing
	// For now, let's stick to simple FindOne
	var reel models.Reel
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&reel)
	if err != nil {
		return nil, err
	}
	return &reel, nil
}

func (r *ReelRepository) ListReels(ctx context.Context, limit int64, offset int64) ([]models.Reel, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit).SetSkip(offset)

	cur, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var reels []models.Reel
	if err := cur.All(ctx, &reels); err != nil {
		return nil, err
	}
	return reels, nil
}

func (r *ReelRepository) GetUserReels(ctx context.Context, userID primitive.ObjectID) ([]models.Reel, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cur, err := r.collection.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var reels []models.Reel
	if err := cur.All(ctx, &reels); err != nil {
		return nil, err
	}
	return reels, nil
}

func (r *ReelRepository) DeleteReel(ctx context.Context, id primitive.ObjectID, userID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	return err
}

func (r *ReelRepository) IncrementViews(ctx context.Context, id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"views": 1}})
	return err
}
