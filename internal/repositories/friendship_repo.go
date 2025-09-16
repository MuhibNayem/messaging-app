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
	existing, err := r.db.Collection("friendships").CountDocuments(ctx, bson.M{
		"$or": []bson.M{
			{
				"requester_id": requesterID,
				"receiver_id": receiverID,
			},
			{
				"requester_id": receiverID,
				"receiver_id": requesterID,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrFriendRequestExists
	}

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
				"receiver_id": userID2,
			},
			{
				"requester_id": userID2,
				"receiver_id": userID1,
			},
		},
	})
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetFriendRequests retrieves pending friend requests with direction filtering
func (r *FriendshipRepository) GetFriendRequests(ctx context.Context, userID primitive.ObjectID, direction string, page, limit int64) ([]models.Friendship, int64, error) {
    log.Printf("[FriendshipRepository] GetFriendRequests called with userID: %s, direction: %s, page: %d, limit: %d", userID.Hex(), direction, page, limit)

    // Build filter based on request direction
    filter := bson.M{
        "status": models.FriendshipStatusPending,
    }

    switch direction {
    case "incoming":
        filter["receiver_id"] = userID
    case "outgoing":
        filter["requester_id"] = userID
    default: // "all" or empty - get both incoming and outgoing
        filter["$or"] = []bson.M{
            {"receiver_id": userID},
            {"requester_id": userID},
        }
    }

    log.Printf("[FriendshipRepository] Using filter: %+v", filter)

    // Get total count for pagination
    total, err := r.db.Collection("friendships").CountDocuments(ctx, filter)
    if err != nil {
        log.Printf("[FriendshipRepository] Error counting documents: %v", err)
        return nil, 0, fmt.Errorf("failed to count requests: %w", err)
    }
    log.Printf("[FriendshipRepository] Total requests found: %d", total)

    // Apply pagination and sorting
    opts := options.Find().
        SetSkip((page - 1) * limit).
        SetLimit(limit).
        SetSort(bson.D{{Key: "created_at", Value: -1}})

    cursor, err := r.db.Collection("friendships").Find(ctx, filter, opts)
    if err != nil {
        log.Printf("[FriendshipRepository] Error finding documents: %v", err)
        return nil, 0, fmt.Errorf("failed to find requests: %w", err)
    }
    defer cursor.Close(ctx)

    var requests []models.Friendship
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
                "receiver_id": friendID,
            },
            {
                "requester_id": friendID,
                "receiver_id": userID,
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
                    "receiver_id": blockedID,
                },
                {
                    "requester_id": blockedID,
                    "receiver_id": blockerID,
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

// Custom errors
var (
	ErrCannotFriendSelf      = errors.New("cannot send friend request to yourself")
	ErrFriendRequestExists   = errors.New("friend request already exists between these users")
	ErrFriendRequestNotFound = errors.New("friend request not found or not actionable")
	ErrCannotBlockSelf = errors.New("cannot block yourself")
    ErrAlreadyBlocked = errors.New("user is already blocked")
	ErrFriendshipNotFound = errors.New("friendship not found")
    ErrBlockNotFound      = errors.New("block relationship not found")
	ErrNotFriends = errors.New("users are not friends")
)
