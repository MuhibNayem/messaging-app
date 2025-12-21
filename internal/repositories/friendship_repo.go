package repositories

import (
	"context"
	"errors"
	"fmt"
	"log"
	"messaging-app/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FriendshipRepository struct {
	db *mongo.Database
}

func NewFriendshipRepository(db *mongo.Database) *FriendshipRepository {
	indexes := []mongo.IndexModel{
		// Unique compound index to prevent duplicate requests in either direction
		{
			Keys: bson.D{
				{Key: "requester_id", Value: 1},
				{Key: "receiver_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		// Index for quick lookup of all requests involving a user
		{
			Keys: bson.D{{Key: "$**", Value: "text"}}, // Wildcard index for flexible queries
		},
		// TTL index for auto-expiring pending requests after 30 days
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(30 * 24 * 60 * 60),
		},
	}

	_, err := db.Collection("friendships").Indexes().CreateMany(context.Background(), indexes)
	if err != nil {
		panic("Failed to create friendship indexes: " + err.Error())
	}

	return &FriendshipRepository{db: db}
}

// CreateRequest creates a new friend request with conflict prevention
func (r *FriendshipRepository) CreateRequest(ctx context.Context, requesterID, receiverID primitive.ObjectID) (*models.Friendship, error) {
	// Prevent self-friending
	if requesterID == receiverID {
		return nil, ErrCannotFriendSelf
	}

	// Check for existing request in either direction
	var existingFriendship models.Friendship
	err := r.db.Collection("friendships").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{
				"requester_id": requesterID,
				"receiver_id":  receiverID,
			},
			{
				"requester_id": receiverID,
				"receiver_id":  requesterID,
			},
		},
	}).Decode(&existingFriendship)

	if err == nil {
		// Found an existing record
		if existingFriendship.Status == models.FriendshipStatusRejected {
			// If rejected, we can reactivate it as a new pending request from the current requester
			_, err := r.db.Collection("friendships").UpdateOne(ctx,
				bson.M{"_id": existingFriendship.ID},
				bson.M{
					"$set": bson.M{
						"requester_id": requesterID,
						"receiver_id":  receiverID,
						"status":       models.FriendshipStatusPending,
						"updated_at":   time.Now(),
					},
				},
			)
			if err != nil {
				return nil, err
			}
			// Return updated friendship
			existingFriendship.Status = models.FriendshipStatusPending
			existingFriendship.RequesterID = requesterID
			existingFriendship.ReceiverID = receiverID
			return &existingFriendship, nil
		}
		// Otherwise (Pending, Accepted, Blocked), it's a conflict
		return nil, ErrFriendRequestExists
	} else if err != mongo.ErrNoDocuments {
		// Real DB error
		return nil, err
	}

	// No existing record, create new
	friendship := &models.Friendship{
		RequesterID: requesterID,
		ReceiverID:  receiverID,
		Status:      models.FriendshipStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result, err := r.db.Collection("friendships").InsertOne(ctx, friendship)
	if err != nil {
		return nil, err
	}

	friendship.ID = result.InsertedID.(primitive.ObjectID)
	return friendship, nil
}

// UpdateStatus updates request status with validation
func (r *FriendshipRepository) UpdateStatus(ctx context.Context, friendshipID primitive.ObjectID, receiverID primitive.ObjectID, status models.FriendshipStatus) error {
	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	// Only the receiver can accept/reject requests
	result, err := r.db.Collection("friendships").UpdateOne(
		ctx,
		bson.M{
			"_id":         friendshipID,
			"receiver_id": receiverID,
			"status":      models.FriendshipStatusPending,
		},
		update,
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return ErrFriendRequestNotFound
	}

	return nil
}

// AreFriends checks if two users have an accepted friendship
func (r *FriendshipRepository) AreFriends(ctx context.Context, userID1, userID2 primitive.ObjectID) (bool, error) {
	count, err := r.db.Collection("friendships").CountDocuments(ctx, bson.M{
		"status": models.FriendshipStatusAccepted,
		"$or": []bson.M{
			{
				"requester_id": userID1,
				"receiver_id":  userID2,
			},
			{
				"requester_id": userID2,
				"receiver_id":  userID1,
			},
		},
	})
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetPendingRequest retrieves a pending friend request between two specific users
func (r *FriendshipRepository) GetPendingRequest(ctx context.Context, requesterID, receiverID primitive.ObjectID) (*models.Friendship, error) {
	var friendship models.Friendship
	err := r.db.Collection("friendships").FindOne(ctx, bson.M{
		"requester_id": requesterID,
		"receiver_id":  receiverID,
		"status":       models.FriendshipStatusPending,
	}).Decode(&friendship)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.New("friend request not found") // Custom error for clarity
		}
		return nil, fmt.Errorf("failed to find pending request: %w", err)
	}
	return &friendship, nil
}

// GetFriendRequests retrieves friend requests with status filtering and populated user data
func (r *FriendshipRepository) GetFriendRequests(ctx context.Context, userID primitive.ObjectID, status models.FriendshipStatus, page, limit int64) ([]models.PopulatedFriendship, int64, error) {
	log.Printf("[FriendshipRepository] GetFriendRequests called with userID: %s, status: %s, page: %d, limit: %d", userID.Hex(), status, page, limit)

	// Match stage to filter friendships by the current user and status
	matchFilter := bson.M{
		"status": status,
		"$or": []bson.M{
			{"requester_id": userID},
			{"receiver_id": userID},
		},
	}
	matchStage := bson.D{{Key: "$match", Value: matchFilter}}

	// Use CountDocuments for a robust total count
	total, err := r.db.Collection("friendships").CountDocuments(ctx, matchFilter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	// Main aggregation pipeline
	pipeline := mongo.Pipeline{
		matchStage,
		// Lookup requester info
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "requester_id",
			"foreignField": "_id",
			"as":           "requester_info",
		}}},
		// Lookup receiver info
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "receiver_id",
			"foreignField": "_id",
			"as":           "receiver_info",
		}}},
		// Unwind the arrays created by lookup
		bson.D{{Key: "$unwind", Value: "$requester_info"}},
		bson.D{{Key: "$unwind", Value: "$receiver_info"}},
		// Project the final structure
		bson.D{{Key: "$project", Value: bson.M{
			"_id":            1,
			"status":         1,
			"created_at":     1,
			"updated_at":     1,
			"requester_id":   1,
			"receiver_id":    1,
			"requester_info": "$requester_info",
			"receiver_info":  "$receiver_info",
		}}},
		// Sorting and pagination
		bson.D{{Key: "$sort", Value: bson.D{{Key: "updated_at", Value: -1}}}},
		bson.D{{Key: "$skip", Value: (page - 1) * limit}},
		bson.D{{Key: "$limit", Value: limit}},
	}

	cursor, err := r.db.Collection("friendships").Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("[FriendshipRepository] Error finding documents: %v", err)
		return nil, 0, fmt.Errorf("failed to find requests: %w", err)
	}
	defer cursor.Close(ctx)

	requests := make([]models.PopulatedFriendship, 0)
	if err := cursor.All(ctx, &requests); err != nil {
		log.Printf("[FriendshipRepository] Error decoding cursor: %v", err)
		return nil, 0, fmt.Errorf("failed to decode requests: %w", err)
	}

	log.Printf("[FriendshipRepository] Successfully retrieved %d friend requests", len(requests))
	return requests, total, nil
}

// Unfriend removes an accepted friendship between two users after verification
func (r *FriendshipRepository) Unfriend(ctx context.Context, userID, friendID primitive.ObjectID) error {
	// First check if they are actually friends
	areFriends, err := r.AreFriends(ctx, userID, friendID)
	if err != nil {
		return fmt.Errorf("failed to verify friendship status: %w", err)
	}
	if !areFriends {
		return ErrNotFriends
	}

	// Delete the friendship record in either direction
	result, err := r.db.Collection("friendships").DeleteOne(ctx, bson.M{
		"status": models.FriendshipStatusAccepted,
		"$or": []bson.M{
			{
				"requester_id": userID,
				"receiver_id":  friendID,
			},
			{
				"requester_id": friendID,
				"receiver_id":  userID,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete friendship: %w", err)
	}

	if result.DeletedCount == 0 {
		// This shouldn't happen since we checked AreFriends first, but handle just in case
		return ErrFriendshipNotFound
	}

	return nil
}

// BlockUser blocks a user (creates a blocked status relationship)
// BlockUser blocks a user with proper verification checks
func (r *FriendshipRepository) BlockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
	// Prevent self-blocking
	if blockerID == blockedID {
		return ErrCannotBlockSelf
	}

	// Check if already blocked
	alreadyBlocked, err := r.IsBlocked(ctx, blockerID, blockedID)
	if err != nil {
		return fmt.Errorf("failed to check block status: %w", err)
	}
	if alreadyBlocked {
		return ErrAlreadyBlocked
	}

	// Check if there's an existing friendship to prevent accidental blocking
	areFriends, err := r.AreFriends(ctx, blockerID, blockedID)
	if err != nil {
		return fmt.Errorf("failed to verify friendship status: %w", err)
	}

	// First remove any existing friendship/request if they were friends
	if areFriends {
		_, err = r.db.Collection("friendships").DeleteMany(ctx, bson.M{
			"$or": []bson.M{
				{
					"requester_id": blockerID,
					"receiver_id":  blockedID,
				},
				{
					"requester_id": blockedID,
					"receiver_id":  blockerID,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to remove existing friendship: %w", err)
		}
	}

	// Create blocked relationship
	blockedFriendship := &models.Friendship{
		RequesterID: blockerID,
		ReceiverID:  blockedID,
		Status:      models.FriendshipStatusBlocked,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = r.db.Collection("friendships").InsertOne(ctx, blockedFriendship)
	if err != nil {
		return fmt.Errorf("failed to create block: %w", err)
	}

	return nil
}

// UnblockUser removes a block between users with proper verification
func (r *FriendshipRepository) UnblockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
	// Check if the block exists
	// Specific check that blockerID blocked blockedID
	isBlocked, err := r.IsBlockedBy(ctx, blockedID, blockerID)
	if err != nil {
		return fmt.Errorf("failed to verify block status: %w", err)
	}
	if !isBlocked {
		return ErrBlockNotFound
	}

	// Delete the specific block relationship
	result, err := r.db.Collection("friendships").DeleteOne(ctx, bson.M{
		"requester_id": blockerID,
		"receiver_id":  blockedID,
		"status":       models.FriendshipStatusBlocked,
	})
	if err != nil {
		return fmt.Errorf("failed to remove block: %w", err)
	}

	if result.DeletedCount == 0 {
		// This shouldn't happen since we checked IsBlocked first, but handle for safety
		return ErrBlockNotFound
	}

	return nil
}

// IsBlockedBy checks if blockerID has specifically blocked blockedID
func (r *FriendshipRepository) IsBlockedBy(ctx context.Context, blockedID, blockerID primitive.ObjectID) (bool, error) {
	count, err := r.db.Collection("friendships").CountDocuments(ctx, bson.M{
		"requester_id": blockerID,
		"receiver_id":  blockedID,
		"status":       models.FriendshipStatusBlocked,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check block relationship: %w", err)
	}
	return count > 0, nil
}

// IsBlocked checks if a user has blocked another user
func (r *FriendshipRepository) IsBlocked(ctx context.Context, userID1, userID2 primitive.ObjectID) (bool, error) {
	count, err := r.db.Collection("friendships").CountDocuments(ctx, bson.M{
		"status": models.FriendshipStatusBlocked,
		"$or": []bson.M{
			{
				"requester_id": userID1,
				"receiver_id":  userID2,
			},
			{
				"requester_id": userID2,
				"receiver_id":  userID1,
			},
		},
	})
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetBlockedUsers returns list of users blocked by the given user
func (r *FriendshipRepository) GetBlockedUsers(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	cursor, err := r.db.Collection("friendships").Find(ctx, bson.M{
		"requester_id": userID,
		"status":       models.FriendshipStatusBlocked,
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var blockedUsers []primitive.ObjectID
	for cursor.Next(ctx) {
		var friendship models.Friendship
		if err := cursor.Decode(&friendship); err != nil {
			return nil, err
		}
		blockedUsers = append(blockedUsers, friendship.ReceiverID)
	}

	return blockedUsers, nil
}

// GetFriends returns a list of users who are friends with the given user
func (r *FriendshipRepository) GetFriends(ctx context.Context, userID primitive.ObjectID) ([]models.User, error) {
	cursor, err := r.db.Collection("friendships").Find(ctx, bson.M{
		"status": models.FriendshipStatusAccepted,
		"$or": []bson.M{
			{"requester_id": userID},
			{"receiver_id": userID},
		},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var friendships []models.Friendship
	if err := cursor.All(ctx, &friendships); err != nil {
		return nil, err
	}

	var friendUsers []models.User
	for _, f := range friendships {
		friendID := f.RequesterID
		if f.RequesterID == userID {
			friendID = f.ReceiverID
		}
		// In a real app, you'd fetch the full user object here from the user collection
		// For now, we'll just return a dummy user with the ID
		friendUsers = append(friendUsers, models.User{ID: friendID})
	}

	return friendUsers, nil
}

// GetFriendIDs returns a list of user IDs who are friends with the given user
func (r *FriendshipRepository) GetFriendIDs(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	cursor, err := r.db.Collection("friendships").Find(ctx, bson.M{
		"status": models.FriendshipStatusAccepted,
		"$or": []bson.M{
			{"requester_id": userID},
			{"receiver_id": userID},
		},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var friendships []models.Friendship
	if err := cursor.All(ctx, &friendships); err != nil {
		return nil, err
	}

	var friendIDs []primitive.ObjectID
	for _, f := range friendships {
		if f.RequesterID == userID {
			friendIDs = append(friendIDs, f.ReceiverID)
		} else {
			friendIDs = append(friendIDs, f.RequesterID)
		}
	}

	return friendIDs, nil
}

// GetPendingFriendshipByID finds a pending friendship by its ID for a specific receiver
func (r *FriendshipRepository) GetPendingFriendshipByID(ctx context.Context, friendshipID, receiverID primitive.ObjectID) (*models.Friendship, error) {
	var friendship models.Friendship
	err := r.db.Collection("friendships").FindOne(ctx, bson.M{
		"_id":         friendshipID,
		"receiver_id": receiverID,
		"status":      models.FriendshipStatusPending,
	}).Decode(&friendship)

	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrFriendRequestNotFound
		}
		return nil, err
	}
	return &friendship, nil
}

// SearchFriends searches for friends whose username or full name matches the query
func (r *FriendshipRepository) SearchFriends(ctx context.Context, userID primitive.ObjectID, query string, limit int64) ([]models.UserShortResponse, error) {
	// Case-insensitive regex for the search query
	regexPattern := fmt.Sprintf(".*%s.*", query)

	pipeline := mongo.Pipeline{
		// 1. Match accepted friendships involving the user
		bson.D{{Key: "$match", Value: bson.M{
			"status": models.FriendshipStatusAccepted,
			"$or": []bson.M{
				{"requester_id": userID},
				{"receiver_id": userID},
			},
		}}},
		// 2. Lookup friend details
		// We need to figure out which field is the "friend" (not the current user)
		// Simpler approach: Lookup both, then pick the right one
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "requester_id",
			"foreignField": "_id",
			"as":           "requester_info",
		}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "receiver_id",
			"foreignField": "_id",
			"as":           "receiver_info",
		}}},
		bson.D{{Key: "$unwind", Value: "$requester_info"}},
		bson.D{{Key: "$unwind", Value: "$receiver_info"}},
		// 3. Project the "friend" info into a common field
		bson.D{{Key: "$project", Value: bson.M{
			"friend_info": bson.M{
				"$cond": bson.A{
					bson.M{"$eq": bson.A{"$requester_id", userID}},
					"$receiver_info",
					"$requester_info",
				},
			},
		}}},
		// 4. Match against the search query
		bson.D{{Key: "$match", Value: bson.M{
			"$or": []bson.M{
				{"friend_info.username": bson.M{"$regex": regexPattern, "$options": "i"}},
				{"friend_info.full_name": bson.M{"$regex": regexPattern, "$options": "i"}},
			},
		}}},
		// 5. Limit results
		bson.D{{Key: "$limit", Value: limit}},
		// 6. Project final shape
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$friend_info"}}},
	}

	cursor, err := r.db.Collection("friendships").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to search friends: %w", err)
	}
	defer cursor.Close(ctx)

	var friends []models.UserShortResponse
	if err := cursor.All(ctx, &friends); err != nil {
		return nil, fmt.Errorf("failed to decode search results: %w", err)
	}

	return friends, nil
}

// Custom errors
var (
	ErrCannotFriendSelf      = errors.New("cannot send friend request to yourself")
	ErrFriendRequestExists   = errors.New("friend request already exists between these users")
	ErrFriendRequestNotFound = errors.New("friend request not found or not actionable")
	ErrCannotBlockSelf       = errors.New("cannot block yourself")
	ErrAlreadyBlocked        = errors.New("user is already blocked")
	ErrFriendshipNotFound    = errors.New("friendship not found")
	ErrBlockNotFound         = errors.New("block relationship not found")
	ErrNotFriends            = errors.New("users are not friends")
)
