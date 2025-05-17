package repositories

import (
	"context"
	"messaging-app/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GroupRepository struct {
	db *mongo.Database
}

func NewGroupRepository(db *mongo.Database) *GroupRepository {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "name", Value: 1}},
			Options: options.Index().SetUnique(false),
		},
		{
			Keys:    bson.D{{Key: "members", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "admins", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
	}

	_, err := db.Collection("groups").Indexes().CreateMany(context.Background(), indexes)
	if err != nil {
		panic("Failed to create group indexes: " + err.Error())
	}

	return &GroupRepository{db: db}
}

func (r *GroupRepository) CreateGroup(ctx context.Context, group *models.Group) (*models.Group, error) {
	group.CreatedAt = time.Now()
	group.UpdatedAt = time.Now()

	// Ensure creator is both admin and member
	if !containsID(group.Admins, group.CreatorID) {
		group.Admins = append(group.Admins, group.CreatorID)
	}
	if !containsID(group.Members, group.CreatorID) {
		group.Members = append(group.Members, group.CreatorID)
	}

	result, err := r.db.Collection("groups").InsertOne(ctx, group)
	if err != nil {
		return nil, err
	}
	group.ID = result.InsertedID.(primitive.ObjectID)
	return group, nil
}

func (r *GroupRepository) GetGroup(ctx context.Context, id primitive.ObjectID) (*models.Group, error) {
	var group models.Group
	err := r.db.Collection("groups").FindOne(ctx, bson.M{"_id": id}).Decode(&group)
	return &group, err
}

func (r *GroupRepository) AddMember(ctx context.Context, groupID, userID primitive.ObjectID) error {
	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{
			"$addToSet": bson.M{"members": userID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

func (r *GroupRepository) AddAdmin(ctx context.Context, groupID, userID primitive.ObjectID) error {
	// First ensure user is a member
	if err := r.AddMember(ctx, groupID, userID); err != nil {
		return err
	}

	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{
			"$addToSet": bson.M{"admins": userID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

func (r *GroupRepository) RemoveMember(ctx context.Context, groupID, userID primitive.ObjectID) error {
	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{
			"$pull": bson.M{
				"members": userID,
				"admins":  userID,
			},
			"$set": bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

func (r *GroupRepository) UpdateGroup(ctx context.Context, groupID primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{"$set": update},
	)
	return err
}


func (r *GroupRepository) GetUserGroups(ctx context.Context, userID primitive.ObjectID) ([]*models.Group, error) {
	groups := []*models.Group{}
	cursor, err := r.db.Collection("groups").Find(ctx, bson.M{"members": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	return groups, err
}

// Helper function
func containsID(ids []primitive.ObjectID, id primitive.ObjectID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}