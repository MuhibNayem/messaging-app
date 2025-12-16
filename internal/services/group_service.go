package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"messaging-app/internal/kafka"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GroupService struct {
	groupRepo *repositories.GroupRepository
	userRepo  *repositories.UserRepository
	producer  *kafka.MessageProducer
}

func NewGroupService(groupRepo *repositories.GroupRepository, userRepo *repositories.UserRepository, producer *kafka.MessageProducer) *GroupService {
	return &GroupService{
		groupRepo: groupRepo,
		userRepo:  userRepo,
		producer:  producer,
	}
}

func (s *GroupService) CreateGroup(ctx context.Context, creatorID primitive.ObjectID, name string, avatar string, memberIDs []primitive.ObjectID) (*models.Group, error) {
	// Verify creator exists
	if _, err := s.userRepo.FindUserByID(ctx, creatorID); err != nil {
		return nil, fmt.Errorf("creator user not found")
	}

	// Verify all members exist
	for _, memberID := range memberIDs {
		if _, err := s.userRepo.FindUserByID(ctx, memberID); err != nil {
			return nil, fmt.Errorf("member %s not found", memberID.Hex())
		}
	}

	// Include creator as admin and member
	members := append([]primitive.ObjectID{}, memberIDs...)
	if !containsID(members, creatorID) {
		members = append(members, creatorID)
	}

	group := &models.Group{
		Name:      name,
		Avatar:    avatar,
		CreatorID: creatorID,
		Members:   members,
		Admins:    []primitive.ObjectID{creatorID},
	}

	// Create group in repository
	createdGroup, err := s.groupRepo.CreateGroup(ctx, group)
	if err != nil {
		return nil, err
	}

	// Publish GROUP_CREATED event
	if err := s.publishGroupEvent(ctx, createdGroup.ID, "GROUP_CREATED"); err != nil {
		fmt.Printf("Failed to publish group created event: %v\n", err)
		// Don't fail the request if event publishing fails
	}
	return createdGroup, nil
}

func (s *GroupService) GetGroup(ctx context.Context, id primitive.ObjectID) (*models.Group, error) {
	return s.groupRepo.GetGroup(ctx, id)
}

func (s *GroupService) AddMember(ctx context.Context, groupID, requesterID, newMemberID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if requester is admin
	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can add members")
	}

	// Check if user is already a member
	if containsID(group.Members, newMemberID) {
		return errors.New("user is already a group member")
	}

	// Verify new member exists
	if _, err := s.userRepo.FindUserByID(ctx, newMemberID); err != nil {
		return fmt.Errorf("user not found")
	}

	return s.groupRepo.AddMember(ctx, groupID, newMemberID)
}

func (s *GroupService) AddAdmin(ctx context.Context, groupID, requesterID, newAdminID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if requester is admin
	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can add other admins")
	}

	// Check if user is already an admin
	if containsID(group.Admins, newAdminID) {
		return errors.New("user is already an admin")
	}

	// Check if user is a member
	if !containsID(group.Members, newAdminID) {
		return errors.New("user must be a member before becoming an admin")
	}

	return s.groupRepo.AddAdmin(ctx, groupID, newAdminID)
}

func (s *GroupService) RemoveMember(ctx context.Context, groupID, requesterID, memberID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if requester is admin
	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can remove members")
	}

	// Check if trying to remove last admin
	if containsID(group.Admins, memberID) && len(group.Admins) == 1 {
		return errors.New("cannot remove the last admin")
	}

	if err := s.groupRepo.RemoveMember(ctx, groupID, memberID); err != nil {
		return err
	}

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) UpdateGroup(ctx context.Context, groupID, requesterID primitive.ObjectID, updates map[string]interface{}) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if requester is admin
	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can update group")
	}

	// Filter allowed fields to update
	allowedFields := map[string]bool{
		"name":       true,
		"avatar":     true,
		"updated_at": true,
	}

	filteredUpdates := bson.M{}
	for key, value := range updates {
		if allowedFields[key] {
			filteredUpdates[key] = value
		}
	}

	if len(filteredUpdates) == 0 {
		return errors.New("no valid fields to update")
	}

	if err := s.groupRepo.UpdateGroup(ctx, groupID, filteredUpdates); err != nil {
		return err
	}

	// Fetch updated group to broadcast
	updatedGroup, err := s.groupRepo.GetGroup(ctx, groupID)
	if err == nil {
		// Broadcast update
		event := map[string]interface{}{
			"type": "GROUP_UPDATED",
			"data": updatedGroup,
		}

		eventBytes, err := json.Marshal(event)
		if err == nil {
			msg := kafkago.Message{
				Key:   []byte(groupID.Hex()),
				Value: eventBytes,
			}
			if err := s.producer.ProduceMessage(ctx, msg); err != nil {
				fmt.Printf("Failed to publish group update event: %v\n", err)
			}
		}
	}

	return nil
}

func (s *GroupService) GetUserGroups(ctx context.Context, userID primitive.ObjectID) ([]*models.Group, error) {
	if _, err := s.userRepo.FindUserByID(ctx, userID); err != nil {
		return nil, fmt.Errorf("user not found")
	}
	groups, err := s.groupRepo.GetUserGroups(ctx, userID)
	return groups, err
}

func (s *GroupService) InviteMember(ctx context.Context, groupID, inviterID, inviteeID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	// Check if inviter is member
	if !containsID(group.Members, inviterID) {
		return errors.New("inviter must be a group member")
	}

	// Check if invitee is already member
	if containsID(group.Members, inviteeID) {
		return errors.New("user is already a member")
	}
	// Check if invitee is already pending
	if containsID(group.PendingMembers, inviteeID) {
		return errors.New("user is already pending approval")
	}

	// Logic:
	// If Inviter is Admin -> Add Immediate
	// If Settings.RequiresApproval is FALSE -> Add Immediate
	// Else -> Add Pending

	isInviterAdmin := containsID(group.Admins, inviterID)
	requiresApproval := group.Settings.RequiresApproval

	if isInviterAdmin || !requiresApproval {
		return s.groupRepo.AddMember(ctx, groupID, inviteeID)
	}

	// Add to pending
	return s.groupRepo.AddPendingMember(ctx, groupID, inviteeID)
}

func (s *GroupService) ApproveMember(ctx context.Context, groupID, adminID, targetUserID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, adminID) {
		return errors.New("only admins can approve members")
	}

	if !containsID(group.PendingMembers, targetUserID) {
		return errors.New("user is not in pending list")
	}

	// Remove from pending
	if err := s.groupRepo.RemovePendingMember(ctx, groupID, targetUserID); err != nil {
		return err
	}
	// Add to members
	if err := s.groupRepo.AddMember(ctx, groupID, targetUserID); err != nil {
		return err
	}

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) RejectMember(ctx context.Context, groupID, adminID, targetUserID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, adminID) {
		return errors.New("only admins can reject members")
	}

	if err := s.groupRepo.RemovePendingMember(ctx, groupID, targetUserID); err != nil {
		return err
	}

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) RemoveAdmin(ctx context.Context, groupID, requesterID, adminID primitive.ObjectID) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can remove admins")
	}

	if len(group.Admins) <= 1 {
		return errors.New("cannot remove the last admin")
	}

	// To remove admin (demote), we assume the repository has a RemoveAdmin method
	// NOTE: existing RemoveMember removes from BOTH.
	// We need a specific RemoveAdmin (demote) repo function or custom update.
	// Since we can't change Repo structure easily repeatedly, let's implement demote logic here via generic Update if possible?
	// But Repo UpdateGroup is generic.
	// Better: Add RemoveAdminRole to Repo?
	// Or use generic UpdateGroup with $pull from admins array.
	// Actually GroupRepo has UpdateGroup. we can use that.

	// Use the dedicated RemoveAdmin repository method
	if err := s.groupRepo.RemoveAdmin(ctx, groupID, adminID); err != nil {
		return err
	}

	// Publish update event
	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func (s *GroupService) publishGroupEvent(ctx context.Context, groupID primitive.ObjectID, eventType string) error {
	// 1. Fetch latest group state
	updatedGroup, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("failed to fetch group for broadcast: %w", err)
	}

	// 2. Fetch User Details for Enrichment
	// We need to construct models.GroupResponse

	// Helper to fetch user details safely
	getUserShort := func(uid primitive.ObjectID) (models.UserShortResponse, error) {
		u, err := s.userRepo.FindUserByID(ctx, uid)
		if err != nil {
			return models.UserShortResponse{}, err
		}
		return models.UserShortResponse{
			ID:       u.ID,
			Username: u.Username,
			Email:    u.Email,
			Avatar:   u.Avatar,
		}, nil
	}

	// A. Creator
	creator, err := getUserShort(updatedGroup.CreatorID)
	if err != nil {
		// Log error but proceed? Or fail? Better to fail or send partial?
		// For broadcast, maybe better to proceed with empty or fail.
		// Let's try to be robust.
		fmt.Printf("Error fetching creator for broadcast: %v\n", err)
	}

	// B. Members
	var members []models.UserShortResponse
	for _, mid := range updatedGroup.Members {
		if u, err := getUserShort(mid); err == nil {
			members = append(members, u)
		}
	}

	// C. Pending Members
	var pendingMembers []models.UserShortResponse
	for _, pid := range updatedGroup.PendingMembers {
		if u, err := getUserShort(pid); err == nil {
			pendingMembers = append(pendingMembers, u)
		}
	}

	// D. Admins
	var admins []models.UserShortResponse
	for _, aid := range updatedGroup.Admins {
		if u, err := getUserShort(aid); err == nil {
			admins = append(admins, u)
		}
	}

	// 3. Construct Response
	response := models.GroupResponse{
		ID:             updatedGroup.ID,
		Name:           updatedGroup.Name,
		Avatar:         updatedGroup.Avatar,
		Creator:        creator,
		Members:        members,
		PendingMembers: pendingMembers,
		Admins:         admins,
		Settings:       updatedGroup.Settings,
		CreatedAt:      updatedGroup.CreatedAt,
		UpdatedAt:      updatedGroup.UpdatedAt,
	}

	// 4. Publish event to Kafka
	// Log the admins list (enriched) to verify
	// fmt.Printf("Broadcasting ENRICHED %s for group %s. Admins Count: %d\n", eventType, updatedGroup.ID.Hex(), len(admins))

	eventPayload := map[string]interface{}{
		"type": eventType, // Dynamic event type
		"data": response,  // The enriched response object
	}

	eventBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal group event: %w", err)
	}

	// Produce to "feed" topic which ws.go consumes
	msg := kafkago.Message{
		Key:   []byte(groupID.Hex()),
		Value: eventBytes,
		Time:  time.Now(),
	}

	if err := s.producer.ProduceMessage(ctx, msg); err != nil {
		fmt.Printf("Failed to publish group event: %v\n", err)
		return err
	}
	return nil
}

func (s *GroupService) UpdateGroupSettings(ctx context.Context, groupID, requesterID primitive.ObjectID, settings models.GroupSettings) error {
	group, err := s.groupRepo.GetGroup(ctx, groupID)
	if err != nil {
		return fmt.Errorf("group not found")
	}

	if !containsID(group.Admins, requesterID) {
		return errors.New("only admins can update settings")
	}

	if err := s.groupRepo.UpdateGroupSettings(ctx, groupID, settings); err != nil {
		return err
	}

	return s.publishGroupEvent(ctx, groupID, "GROUP_UPDATED")
}

func containsID(ids []primitive.ObjectID, id primitive.ObjectID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}
