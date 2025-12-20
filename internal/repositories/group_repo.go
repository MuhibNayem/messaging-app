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

// GetGroupsByIDs retrieves multiple groups by their IDs in a single query
func (r *GroupRepository) GetGroupsByIDs(ctx context.Context, ids []primitive.ObjectID) ([]*models.Group, error) {
	if len(ids) == 0 {
		return []*models.Group{}, nil
	}
	filter := bson.M{"_id": bson.M{"$in": ids}}

	cursor, err := r.db.Collection("groups").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var groups []*models.Group
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	return groups, nil
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

func (r *GroupRepository) RemoveAdmin(ctx context.Context, groupID, userID primitive.ObjectID) error {
	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{
			"$pull": bson.M{"admins": userID},
			"$set":  bson.M{"updated_at": time.Now()},
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

func (r *GroupRepository) AddPendingMember(ctx context.Context, groupID, userID primitive.ObjectID) error {
	// Try adding to set. If pending_members is null, this will fail.
	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{
			"$addToSet": bson.M{"pending_members": userID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
	)

	// If error indicates null field, initialize it
	if err != nil {
		// Just try simpler approach: use $set if null (via query check or just force set if failed)
		// Since we know it failed, we can force reset if needed, but risky if concurrent.
		// Better: FindOne and Update if pending_members is null?

		// Actually simplest fix for the specific user error:
		// Use an aggregation pipeline update which can handle conditionals (Mongo 4.2+)
		// But let's stick to simple retry logic for this specific corruption case.
		_, updateErr := r.db.Collection("groups").UpdateOne(
			ctx,
			bson.M{"_id": groupID, "pending_members": nil},
			bson.M{
				"$set": bson.M{
					"pending_members": []primitive.ObjectID{userID},
					"updated_at":      time.Now(),
				},
			},
		)
		if updateErr == nil {
			return nil // Recovered
		}
		// If updateErr has error, return the original error if secondary failed too
	}
	return err
}

func (r *GroupRepository) RemovePendingMember(ctx context.Context, groupID, userID primitive.ObjectID) error {
	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{
			"$pull": bson.M{"pending_members": userID},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

func (r *GroupRepository) UpdateGroupSettings(ctx context.Context, groupID primitive.ObjectID, settings models.GroupSettings) error {
	_, err := r.db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupID},
		bson.M{
			"$set": bson.M{
				"settings":   settings,
				"updated_at": time.Now(),
			},
		},
	)
	return err
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
