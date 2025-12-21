package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"messaging-app/internal/kafka"
	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/repositories"
	"gitlab.com/spydotech-group/shared-entity/events"
	"time"

	kafkalib "github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FriendshipService struct {
	friendshipRepo *repositories.FriendshipRepository
	userRepo       *repositories.UserRepository
	userGraphRepo  *repositories.UserGraphRepository
	kafkaProducer  *kafka.MessageProducer
}

func NewFriendshipService(fr *repositories.FriendshipRepository, ur *repositories.UserRepository, ugr *repositories.UserGraphRepository, kp *kafka.MessageProducer) *FriendshipService {
	return &FriendshipService{
		friendshipRepo: fr,
		userRepo:       ur,
		userGraphRepo:  ugr,
		kafkaProducer:  kp,
	}
}

func (s *FriendshipService) publishEvent(ctx context.Context, requesterID, receiverID string, status, action string) {
	if s.kafkaProducer == nil {
		return
	}
	event := events.FriendshipEvent{
		RequesterID: requesterID,
		ReceiverID:  receiverID,
		Status:      status,
		Action:      action,
		Timestamp:   time.Now(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal friendship event: %v", err)
		return
	}

	// Use RequesterID as key to partition
	msg := kafkalib.Message{
		Key:   []byte(requesterID),
		Value: payload,
		Time:  time.Now(),
	}

	if err := s.kafkaProducer.ProduceMessage(ctx, msg); err != nil {
		log.Printf("Failed to publish friendship event: %v", err)
	}
}

var (
	ErrCannotFriendSelf      = repositories.ErrCannotFriendSelf
	ErrFriendRequestExists   = repositories.ErrFriendRequestExists
	ErrFriendRequestNotFound = repositories.ErrFriendRequestNotFound
	ErrNotAuthorized         = errors.New("not authorized to perform this action")
)

func (s *FriendshipService) SendRequest(ctx context.Context, requesterID, receiverID primitive.ObjectID) (*models.Friendship, error) {
	// Synch users to graph just in case (if enabled)
	if s.userGraphRepo != nil {
		_ = s.userGraphRepo.SyncUser(ctx, requesterID)
		_ = s.userGraphRepo.SyncUser(ctx, receiverID)
	}

	// Graph: Check existing
	// Note: We can implement AreFriends / RequestExists using Graph check here.

	// Mongo: Create Request (Legacy/Backup)
	friendship, err := s.friendshipRepo.CreateRequest(ctx, requesterID, receiverID)
	if err != nil {
		return nil, err
	}

	// Graph: Create Request
	if s.userGraphRepo != nil {
		if err := s.userGraphRepo.SendRequest(ctx, requesterID, receiverID); err != nil {
			log.Printf("Error creating graph request: %v", err)
			// Don't fail the request if graph fails in dev/migration phase?
			// User said "migrate logic", so maybe we should ensure consistency?
			// For now, log error is safe.
		}
	}

	// Event: Publish Request Sent
	go s.publishEvent(context.Background(), requesterID.Hex(), receiverID.Hex(), "pending", "request")

	return friendship, nil
}

func (s *FriendshipService) RespondToRequest(ctx context.Context, friendshipID primitive.ObjectID, receiverID primitive.ObjectID, accept bool) error {
	log.Printf("Service: RespondToRequest called for friendshipID: %s, receiverID: %s, accept: %t", friendshipID.Hex(), receiverID.Hex(), accept)

	// Mongo: Get request to know who the requester is
	targetRequest, err := s.friendshipRepo.GetPendingFriendshipByID(ctx, friendshipID, receiverID)
	if err != nil {
		return err
	}

	// Mongo: Update Status
	status := models.FriendshipStatusRejected
	if accept {
		status = models.FriendshipStatusAccepted
		if err := s.userRepo.AddFriend(ctx, targetRequest.RequesterID, targetRequest.ReceiverID); err != nil {
			return err
		}
		if err := s.userRepo.AddFriend(ctx, targetRequest.ReceiverID, targetRequest.RequesterID); err != nil {
			_ = s.userRepo.RemoveFriend(ctx, targetRequest.RequesterID, targetRequest.ReceiverID)
			return err
		}
	} else {
		log.Printf("Service: RespondToRequest - Request rejected")
	}

	if err := s.friendshipRepo.UpdateStatus(ctx, friendshipID, receiverID, status); err != nil {
		return err
	}

	// Graph: Update Status
	if s.userGraphRepo != nil {
		if accept {
			if err := s.userGraphRepo.AcceptRequest(ctx, targetRequest.RequesterID, targetRequest.ReceiverID); err != nil {
				log.Printf("Error accepting graph request: %v", err)
			}
		} else {
			if err := s.userGraphRepo.RejectRequest(ctx, targetRequest.RequesterID, targetRequest.ReceiverID); err != nil {
				log.Printf("Error rejecting graph request: %v", err)
			}
		}
	}

	// Event: Publish Status Change
	action := "reject"
	statusStr := "rejected"
	if accept {
		action = "accept"
		statusStr = "accepted"
	}
	go s.publishEvent(context.Background(), targetRequest.RequesterID.Hex(), targetRequest.ReceiverID.Hex(), statusStr, action)

	return nil
}

func (s *FriendshipService) ListFriendships(ctx context.Context, userID primitive.ObjectID, status models.FriendshipStatus, page, limit int64) ([]models.PopulatedFriendship, int64, error) {
	return s.friendshipRepo.GetFriendRequests(ctx, userID, status, page, limit)
}

func (s *FriendshipService) CheckFriendship(ctx context.Context, userID1, userID2 primitive.ObjectID) (bool, error) {
	// Use Graph for fast check if available
	if s.userGraphRepo != nil {
		areFriends, _, _, _, _, err := s.userGraphRepo.CheckFriendshipStatus(ctx, userID1, userID2)
		if err == nil {
			return areFriends, nil
		}
		log.Printf("Graph Check failed, falling back to Mongo: %v", err)
	}
	// Fallback
	return s.friendshipRepo.AreFriends(ctx, userID1, userID2)
}

// Unfriend removes a friendship between two users after validation
func (s *FriendshipService) Unfriend(ctx context.Context, userID, friendID primitive.ObjectID) error {
	// Determine if valid friend (Keep Mongo logic for validation if desired, or trust Graph)

	// Mongo Cleanup
	_ = s.userRepo.RemoveFriend(ctx, userID, friendID)
	_ = s.userRepo.RemoveFriend(ctx, friendID, userID)
	_ = s.friendshipRepo.Unfriend(ctx, userID, friendID)

	// Graph Cleanup
	if s.userGraphRepo != nil {
		return s.userGraphRepo.Unfriend(ctx, userID, friendID)
	}

	// Event: Publish Unfriend
	go s.publishEvent(context.Background(), userID.Hex(), friendID.Hex(), "removed", "remove")

	return nil
}

// BlockUser blocks another user with comprehensive validation
func (s *FriendshipService) BlockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
	// Mongo Block
	if err := s.friendshipRepo.BlockUser(ctx, blockerID, blockedID); err != nil {
		if err != repositories.ErrAlreadyBlocked { // If mongo says already blocked, maybe graph isn't, so continue
			return err
		}
	}

	// Graph Block (Removes friends/requests automatically via Cypher)
	if s.userGraphRepo != nil {
		return s.userGraphRepo.BlockUser(ctx, blockerID, blockedID)
	}

	// Event: Publish Block
	go s.publishEvent(context.Background(), blockerID.Hex(), blockedID.Hex(), "blocked", "block")

	return nil
}

// UnblockUser removes a block between users with validation
func (s *FriendshipService) UnblockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
	// Mongo Unblock
	_ = s.friendshipRepo.UnblockUser(ctx, blockerID, blockedID)

	// Graph Unblock
	if s.userGraphRepo != nil {
		return s.userGraphRepo.UnblockUser(ctx, blockerID, blockedID)
	}

	// Event: Publish Unblock
	go s.publishEvent(context.Background(), blockerID.Hex(), blockedID.Hex(), "unblocked", "unblock")

	return nil
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

// FriendshipStatusResponse defines the detailed status between two users
type FriendshipStatusResponse struct {
	AreFriends        bool `json:"are_friends"`
	RequestSent       bool `json:"request_sent"`         // Viewer sent request to other user
	RequestReceived   bool `json:"request_received"`     // Viewer received request from other user
	IsBlockedByViewer bool `json:"is_blocked_by_viewer"` // Viewer has blocked other user
	HasBlockedViewer  bool `json:"has_blocked_viewer"`   // Other user has blocked viewer
}

// GetDetailedFriendshipStatus provides a comprehensive status between two users
func (s *FriendshipService) GetDetailedFriendshipStatus(ctx context.Context, viewerID, otherUserID primitive.ObjectID) (*FriendshipStatusResponse, error) {
	status := &FriendshipStatusResponse{}

	// Check if they are friends
	areFriends, err := s.friendshipRepo.AreFriends(ctx, viewerID, otherUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to check if friends: %w", err)
	}
	status.AreFriends = areFriends

	// Check if viewer sent a request to other user
	requestSent, err := s.friendshipRepo.GetPendingRequest(ctx, viewerID, otherUserID)
	if err != nil && err.Error() != "friend request not found" {
		return nil, fmt.Errorf("failed to check sent request: %w", err)
	}
	status.RequestSent = (requestSent != nil)

	// Check if viewer received a request from other user
	requestReceived, err := s.friendshipRepo.GetPendingRequest(ctx, otherUserID, viewerID)
	if err != nil && err.Error() != "friend request not found" {
		return nil, fmt.Errorf("failed to check received request: %w", err)
	}
	status.RequestReceived = (requestReceived != nil)

	// Check if viewer has blocked other user
	isBlockedByViewer, err := s.friendshipRepo.IsBlockedBy(ctx, viewerID, otherUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to check if blocked by viewer: %w", err)
	}
	status.IsBlockedByViewer = isBlockedByViewer

	// Check if other user has blocked viewer
	hasBlockedViewer, err := s.friendshipRepo.IsBlockedBy(ctx, otherUserID, viewerID)
	if err != nil {
		return nil, fmt.Errorf("failed to check if has blocked viewer: %w", err)
	}
	status.HasBlockedViewer = hasBlockedViewer

	return status, nil
}

// SearchFriends searches for accepted friends matching the query
func (s *FriendshipService) SearchFriends(ctx context.Context, userID primitive.ObjectID, query string, limit int64) ([]models.UserShortResponse, error) {
	return s.friendshipRepo.SearchFriends(ctx, userID, query, limit)
}

// error declarations
var (
	ErrNotFriends      = repositories.ErrNotFriends
	ErrCannotBlockSelf = repositories.ErrCannotBlockSelf
	ErrAlreadyBlocked  = repositories.ErrAlreadyBlocked
	ErrBlockNotFound   = repositories.ErrBlockNotFound
)
