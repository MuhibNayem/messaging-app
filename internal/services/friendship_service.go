package services

import (
	"context"
	"errors"
	"fmt"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FriendshipService struct {
	friendshipRepo *repositories.FriendshipRepository
	userRepo       *repositories.UserRepository
}

func NewFriendshipService(fr *repositories.FriendshipRepository, ur *repositories.UserRepository) *FriendshipService {
	return &FriendshipService{
		friendshipRepo: fr,
		userRepo:       ur,
	}
}

var (
	ErrCannotFriendSelf      = repositories.ErrCannotFriendSelf
	ErrFriendRequestExists   = repositories.ErrFriendRequestExists
	ErrFriendRequestNotFound = repositories.ErrFriendRequestNotFound
	ErrNotAuthorized         = errors.New("not authorized to perform this action")
)

func (s *FriendshipService) SendRequest(ctx context.Context, requesterID, receiverID primitive.ObjectID) (*models.Friendship, error) {
	// Check if receiver exists
	if _, err := s.userRepo.FindUserByID(ctx, receiverID); err != nil {
		return nil, err // Will return "user not found" from userRepo
	}

	// Check if they are already friends
	if friends, _ := s.friendshipRepo.AreFriends(ctx, requesterID, receiverID); friends {
		return nil, repositories.ErrFriendRequestExists
	}

	// Repository handles all other validation (self-friending, existing requests)
	return s.friendshipRepo.CreateRequest(ctx, requesterID, receiverID)
}

func (s *FriendshipService) RespondToRequest(ctx context.Context, friendshipID primitive.ObjectID, receiverID primitive.ObjectID, accept bool) error {
	// First verify the request exists and belongs to this user
	friendships, _, err := s.friendshipRepo.GetFriendRequests(ctx, receiverID, models.FriendshipStatusPending, 1, 1)
	if err != nil {
		return err
	}

	var targetRequest *models.Friendship
	for _, f := range friendships {
		if f.ID == friendshipID {
			targetRequest = &f
			break
		}
	}

	if targetRequest == nil {
		return repositories.ErrFriendRequestNotFound
	}

	if targetRequest.ReceiverID != receiverID {
		return repositories.ErrFriendRequestNotFound 
	}

	status := models.FriendshipStatusRejected
	if accept {
		status = models.FriendshipStatusAccepted
		// Update both users' friend lists
		if err := s.userRepo.AddFriend(ctx, targetRequest.RequesterID, targetRequest.ReceiverID); err != nil {
			return err
		}
		if err := s.userRepo.AddFriend(ctx, targetRequest.ReceiverID, targetRequest.RequesterID); err != nil {
			return err
		}
	}

	return s.friendshipRepo.UpdateStatus(ctx, friendshipID, receiverID, status)
}

func (s *FriendshipService) ListFriendships(ctx context.Context, userID primitive.ObjectID, status string, page, limit int64) ([]models.Friendship, int64, error) {
	return s.friendshipRepo.GetFriendRequests(ctx, userID, status, page, limit)
}

func (s *FriendshipService) CheckFriendship(ctx context.Context, userID1, userID2 primitive.ObjectID) (bool, error) {
	return s.friendshipRepo.AreFriends(ctx, userID1, userID2)
}

// Unfriend removes a friendship between two users after validation
func (s *FriendshipService) Unfriend(ctx context.Context, userID, friendID primitive.ObjectID) error {
    // Verify friend exists
    if _, err := s.userRepo.FindUserByID(ctx, friendID); err != nil {
        return err // Returns "user not found" if friend doesn't exist
    }

    // Check if they are actually friends
    areFriends, err := s.friendshipRepo.AreFriends(ctx, userID, friendID)
    if err != nil {
        return fmt.Errorf("failed to verify friendship: %w", err)
    }
    if !areFriends {
        return repositories.ErrNotFriends
    }

    // Remove from both users' friend lists
    if err := s.friendshipRepo.Unfriend(ctx, userID, friendID); err != nil {
        return fmt.Errorf("failed to remove from friend list: %w", err)
    }
    if err := s.friendshipRepo.Unfriend(ctx, friendID, userID); err != nil {
        return fmt.Errorf("failed to remove reciprocal friend: %w", err)
    }

    // Delete the friendship record
    return s.friendshipRepo.Unfriend(ctx, userID, friendID)
}

// BlockUser blocks another user with comprehensive validation
func (s *FriendshipService) BlockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
    // Verify blocked user exists
    if _, err := s.userRepo.FindUserByID(ctx, blockedID); err != nil {
        return err // Returns "user not found" if blocked user doesn't exist
    }

    // Check if already blocked
    alreadyBlocked, err := s.friendshipRepo.IsBlocked(ctx, blockerID, blockedID)
    if err != nil {
        return fmt.Errorf("failed to check block status: %w", err)
    }
    if alreadyBlocked {
        return repositories.ErrAlreadyBlocked
    }

    // Perform the block
    return s.friendshipRepo.BlockUser(ctx, blockerID, blockedID)
}

// UnblockUser removes a block between users with validation
func (s *FriendshipService) UnblockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
    // Verify blocked user exists
    if _, err := s.userRepo.FindUserByID(ctx, blockedID); err != nil {
        return err // Returns "user not found" if user doesn't exist
    }

    // Check if block exists
    isBlocked, err := s.friendshipRepo.IsBlocked(ctx, blockerID, blockedID)
    if err != nil {
        return fmt.Errorf("failed to verify block: %w", err)
    }
    if !isBlocked {
        return repositories.ErrBlockNotFound
    }

    return s.friendshipRepo.UnblockUser(ctx, blockerID, blockedID)
}

// IsBlocked checks if a block exists between two users
func (s *FriendshipService) IsBlocked(ctx context.Context, userID1, userID2 primitive.ObjectID) (bool, error) {
    // Check both directions since blocks can be mutual
    blocked1, err := s.friendshipRepo.IsBlockedBy(ctx, userID1, userID2)
    if err != nil {
        return false, fmt.Errorf("failed to check block status: %w", err)
    }

    blocked2, err := s.friendshipRepo.IsBlockedBy(ctx, userID2, userID1)
    if err != nil {
        return false, fmt.Errorf("failed to check reciprocal block: %w", err)
    }

    return blocked1 || blocked2, nil
}

// GetBlockedUsers retrieves list of users blocked by the given user
func (s *FriendshipService) GetBlockedUsers(ctx context.Context, userID primitive.ObjectID) ([]models.User, error) {
    blockedIDs, err := s.friendshipRepo.GetBlockedUsers(ctx, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to get blocked users: %w", err)
    }

    // Fetch full user details for each blocked user
    var blockedUsers []models.User
    for _, id := range blockedIDs {
        user, err := s.userRepo.FindUserByID(ctx, id)
        if err != nil {
            // Skip users that might have been deleted
            continue
        }
        blockedUsers = append(blockedUsers, *user)
    }

    return blockedUsers, nil
}

//error declarations
var (
    ErrNotFriends     = repositories.ErrNotFriends
    ErrCannotBlockSelf = repositories.ErrCannotBlockSelf
    ErrAlreadyBlocked = repositories.ErrAlreadyBlocked
    ErrBlockNotFound  = repositories.ErrBlockNotFound
)