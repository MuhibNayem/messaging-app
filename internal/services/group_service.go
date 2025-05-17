package services

import (
	"context"
	"errors"
	"fmt"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GroupService struct {
	groupRepo *repositories.GroupRepository
	userRepo  *repositories.UserRepository
}

func NewGroupService(groupRepo *repositories.GroupRepository, userRepo *repositories.UserRepository) *GroupService {
	return &GroupService{
		groupRepo: groupRepo,
		userRepo:  userRepo,
	}
}

func (s *GroupService) CreateGroup(ctx context.Context, creatorID primitive.ObjectID, name string, memberIDs []primitive.ObjectID) (*models.Group, error) {
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
		CreatorID: creatorID,
		Members:   members,
		Admins:    []primitive.ObjectID{creatorID},
	}

	return s.groupRepo.CreateGroup(ctx, group)
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

	return s.groupRepo.RemoveMember(ctx, groupID, memberID)
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

	return s.groupRepo.UpdateGroup(ctx, groupID, filteredUpdates)
}

func (s *GroupService) GetUserGroups(ctx context.Context, userID primitive.ObjectID) ([]*models.Group, error) {
	if _, err := s.userRepo.FindUserByID(ctx, userID); err != nil {
		return nil, fmt.Errorf("user not found")
	}
	groups, err := s.groupRepo.GetUserGroups(ctx, userID)
	return groups, err
}

func containsID(ids []primitive.ObjectID, id primitive.ObjectID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}