package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
)

// PrivacyService provides business logic for privacy-related operations
type PrivacyService struct {
	privacyRepo repositories.PrivacyRepository
	userRepo    *repositories.UserRepository
}

// NewPrivacyService creates a new instance of PrivacyService
func NewPrivacyService(
	privacyRepo repositories.PrivacyRepository,
	userRepo *repositories.UserRepository,
) *PrivacyService {
	return &PrivacyService{
		privacyRepo: privacyRepo,
		userRepo:    userRepo,
	}
}

// GetUserPrivacySettings retrieves a user's privacy settings
func (s *PrivacyService) GetUserPrivacySettings(ctx context.Context, userID primitive.ObjectID) (*models.UserPrivacySettings, error) {
	settings, err := s.privacyRepo.GetUserPrivacySettings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user privacy settings: %w", err)
	}

	// If no settings exist, return default settings
	if settings == nil {
		return &models.UserPrivacySettings{
			UserID:                  userID,
			DefaultPostPrivacy:      models.PrivacySettingFriends,
			CanSeeMyFriendsList:     models.PrivacySettingFriends,
			CanSendMeFriendRequests: models.PrivacySettingEveryone,
			CanTagMeInPosts:         models.PrivacySettingFriends,
			LastUpdated:             time.Now(),
		}, nil
	}

	return settings, nil
}

// UpdateUserPrivacySettings updates a user's privacy settings
func (s *PrivacyService) UpdateUserPrivacySettings(ctx context.Context, userID primitive.ObjectID, req *models.UpdatePrivacySettingsRequest) (*models.UserPrivacySettings, error) {
	settings, err := s.privacyRepo.GetUserPrivacySettings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user privacy settings: %w", err)
	}

	if settings == nil {
		settings = &models.UserPrivacySettings{
			UserID: userID,
			// Set defaults if creating new settings
			DefaultPostPrivacy:      models.PrivacySettingFriends,
			CanSeeMyFriendsList:     models.PrivacySettingFriends,
			CanSendMeFriendRequests: models.PrivacySettingEveryone,
			CanTagMeInPosts:         models.PrivacySettingFriends,
		}
	}

	// Apply updates
	if req.DefaultPostPrivacy != "" {
		settings.DefaultPostPrivacy = req.DefaultPostPrivacy
	}
	if req.CanSeeMyFriendsList != "" {
		settings.CanSeeMyFriendsList = req.CanSeeMyFriendsList
	}
	if req.CanSendMeFriendRequests != "" {
		settings.CanSendMeFriendRequests = req.CanSendMeFriendRequests
	}
	if req.CanTagMeInPosts != "" {
		settings.CanTagMeInPosts = req.CanTagMeInPosts
	}
	settings.LastUpdated = time.Now()

	if err := s.privacyRepo.CreateOrUpdateUserPrivacySettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("failed to update user privacy settings: %w", err)
	}

	return settings, nil
}

// CreateCustomPrivacyList creates a new custom privacy list for a user
func (s *PrivacyService) CreateCustomPrivacyList(ctx context.Context, userID primitive.ObjectID, req *models.CreateCustomPrivacyListRequest) (*models.CustomPrivacyList, error) {
	// Ensure all members are valid users
	for _, memberID := range req.Members {
		user, err := s.userRepo.FindUserByID(ctx, memberID)
		if err != nil || user == nil {
			return nil, fmt.Errorf("member with ID %s not found", memberID.Hex())
		}
	}

	list := &models.CustomPrivacyList{
		ID:        primitive.NewObjectID(),
		UserID:    userID,
		Name:      req.Name,
		Members:   req.Members,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.privacyRepo.CreateCustomPrivacyList(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to create custom privacy list: %w", err)
	}

	return list, nil
}

// GetCustomPrivacyListByID retrieves a custom privacy list by its ID
func (s *PrivacyService) GetCustomPrivacyListByID(ctx context.Context, listID, userID primitive.ObjectID) (*models.CustomPrivacyList, error) {
	list, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom privacy list: %w", err)
	}
	if list == nil {
		return nil, errors.New("custom privacy list not found")
	}

	if list.UserID != userID {
		return nil, errors.New("not authorized to view this custom privacy list")
	}

	return list, nil
}

// GetCustomPrivacyListsByUserID retrieves all custom privacy lists for a user
func (s *PrivacyService) GetCustomPrivacyListsByUserID(ctx context.Context, userID primitive.ObjectID) ([]models.CustomPrivacyList, error) {
	lists, err := s.privacyRepo.GetCustomPrivacyListsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom privacy lists: %w", err)
	}
	return lists, nil
}

// UpdateCustomPrivacyList updates an existing custom privacy list
func (s *PrivacyService) UpdateCustomPrivacyList(ctx context.Context, listID, userID primitive.ObjectID, req *models.UpdateCustomPrivacyListRequest) (*models.CustomPrivacyList, error) {
	list, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom privacy list: %w", err)
	}
	if list == nil {
		return nil, errors.New("custom privacy list not found")
	}

	if list.UserID != userID {
		return nil, errors.New("not authorized to update this custom privacy list")
	}

	update := bson.M{"$set": bson.M{}}
	if req.Name != "" {
		update["$set"].(bson.M)["name"] = req.Name
	}
	if req.Members != nil {
		// Ensure all new members are valid users
		for _, memberID := range req.Members {
			user, err := s.userRepo.FindUserByID(ctx, memberID)
			if err != nil || user == nil {
				return nil, fmt.Errorf("member with ID %s not found", memberID.Hex())
			}
		}
		update["$set"].(bson.M)["members"] = req.Members
	}

	if len(update["$set"].(bson.M)) == 0 {
		return list, nil // No changes to apply
	}

	if err := s.privacyRepo.UpdateCustomPrivacyList(ctx, listID, update); err != nil {
		return nil, fmt.Errorf("failed to update custom privacy list: %w", err)
	}

	updatedList, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve updated custom privacy list: %w", err)
	}
	return updatedList, nil
}

// DeleteCustomPrivacyList deletes a custom privacy list
func (s *PrivacyService) DeleteCustomPrivacyList(ctx context.Context, listID, userID primitive.ObjectID) error {
	list, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return fmt.Errorf("failed to get custom privacy list: %w", err)
	}
	if list == nil {
		return errors.New("custom privacy list not found")
	}

	if list.UserID != userID {
		return errors.New("not authorized to delete this custom privacy list")
	}

	if err := s.privacyRepo.DeleteCustomPrivacyList(ctx, listID); err != nil {
		return fmt.Errorf("failed to delete custom privacy list: %w", err)
	}
	return nil
}

// AddMemberToCustomPrivacyList adds a member to a custom privacy list
func (s *PrivacyService) AddMemberToCustomPrivacyList(ctx context.Context, listID, userID primitive.ObjectID, memberUserID primitive.ObjectID) (*models.CustomPrivacyList, error) {
	list, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom privacy list: %w", err)
	}
	if list == nil {
		return nil, errors.New("custom privacy list not found")
	}

	if list.UserID != userID {
		return nil, errors.New("not authorized to modify this custom privacy list")
	}

	// Check if memberUserID is a valid user
	memberUser, err := s.userRepo.FindUserByID(ctx, memberUserID)
	if err != nil || memberUser == nil {
		return nil, errors.New("member user not found")
	}

	if err := s.privacyRepo.AddMemberToCustomPrivacyList(ctx, listID, memberUserID); err != nil {
		return nil, fmt.Errorf("failed to add member to custom privacy list: %w", err)
	}

	updatedList, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve updated custom privacy list: %w", err)
	}
	return updatedList, nil
}

// RemoveMemberFromCustomPrivacyList removes a member from a custom privacy list
func (s *PrivacyService) RemoveMemberFromCustomPrivacyList(ctx context.Context, listID, userID primitive.ObjectID, memberUserID primitive.ObjectID) (*models.CustomPrivacyList, error) {
	list, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom privacy list: %w", err)
	}
	if list == nil {
		return nil, errors.New("custom privacy list not found")
	}

	if list.UserID != userID {
		return nil, errors.New("not authorized to modify this custom privacy list")
	}

	if err := s.privacyRepo.RemoveMemberFromCustomPrivacyList(ctx, listID, memberUserID); err != nil {
		return nil, fmt.Errorf("failed to remove member from custom privacy list: %w", err)
	}

	updatedList, err := s.privacyRepo.GetCustomPrivacyListByID(ctx, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve updated custom privacy list: %w", err)
	}
	return updatedList, nil
}
