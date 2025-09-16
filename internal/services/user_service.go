package services

import (
	"context"
	"errors"
	"fmt"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo *repositories.UserRepository
}

func NewUserService(userRepo *repositories.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

func (s *UserService) GetUserByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	return s.userRepo.FindUserByID(ctx, id)
}

func (s *UserService) UpdateUser(ctx context.Context, id primitive.ObjectID, update *models.UserUpdateRequest) (*models.User, error) {
	updateData := bson.M{
		"updated_at": time.Now(),
	}

	if update.Username != "" {
		existingUserUsername, _ := s.userRepo.FindUserByUserName(ctx, update.Username)
		if existingUserUsername != nil && existingUserUsername.ID != id {
			return nil, errors.New("username already exists")
		}
		updateData["username"] = update.Username
	}

	if update.Email != "" {
		existingUserEmail, _ := s.userRepo.FindUserByEmail(ctx, update.Email)
		if existingUserEmail != nil && existingUserEmail.ID != id {
			return nil, errors.New("user email already exists")
		}
		updateData["email"] = update.Email
		updateData["email_verified"] = false // Email needs re-verification
	}

	// Only update password if new password provided
	if update.CurrentPassword != "" && update.NewPassword != "" {
		user, err := s.userRepo.FindUserByID(ctx, id)
		if err != nil {
			return nil, err
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(update.CurrentPassword)); err != nil {
			return nil, errors.New("current password is incorrect")
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(update.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		updateData["password"] = string(hashedPassword)
	}

	if update.FullName != "" {
		updateData["full_name"] = update.FullName
	}
	if update.Bio != "" {
		updateData["bio"] = update.Bio
	}
	if update.DateOfBirth != nil {
		updateData["date_of_birth"] = update.DateOfBirth
	}
	if update.Gender != "" {
		updateData["gender"] = update.Gender
	}
	if update.Location != "" {
		updateData["location"] = update.Location
	}
	if update.PhoneNumber != "" {
		updateData["phone_number"] = update.PhoneNumber
	}
	if update.Avatar != "" {
		updateData["avatar"] = update.Avatar
	}

	updatedUser, err := s.userRepo.UpdateUser(ctx, id, updateData)
	if err != nil {
		return nil, err
	}

	// Clear password before returning
	updatedUser.Password = ""
	return updatedUser, nil
}

func (s *UserService) ListUsers(ctx context.Context, page, limit int64, search string) (*models.UserListResponse, error) {
	filter := bson.M{}
	if search != "" {
		filter["$or"] = []bson.M{
			{"username": bson.M{"$regex": search, "$options": "i"}},
			{"email": bson.M{"$regex": search, "$options": "i"}},
		}
	}

	// Get total count
	total, err := s.userRepo.CountUsers(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Pagination options
	opts := options.Find().
		SetSkip((page - 1) * limit).
		SetLimit(limit).
		SetSort(bson.D{{Key: "username", Value: 1}})

	users, err := s.userRepo.FindUsers(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	// Clear passwords
	for i := range users {
		users[i].Password = ""
	}

	return &models.UserListResponse{
		Users: users,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

// UpdateEmail updates a user's email address
func (s *UserService) UpdateEmail(ctx context.Context, userID primitive.ObjectID, req *models.UpdateEmailRequest) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	// Check if new email already exists
	existingUser, err := s.userRepo.FindUserByEmail(ctx, req.NewEmail)
	if err != nil {
		return err
	}
	if existingUser != nil && existingUser.ID != userID {
		return errors.New("email already in use by another account")
	}

	updateData := bson.M{
		"email":          req.NewEmail,
		"email_verified": false, // New email needs verification
		"updated_at":     time.Now(),
	}

	_, err = s.userRepo.UpdateUser(ctx, userID, updateData)
	if err != nil {
		return fmt.Errorf("failed to update email: %w", err)
	}

	// TODO: Trigger email verification process

	return nil
}

// UpdatePassword updates a user's password
func (s *UserService) UpdatePassword(ctx context.Context, userID primitive.ObjectID, req *models.UpdatePasswordRequest) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	updateData := bson.M{
		"password":   string(hashedPassword),
		"updated_at": time.Now(),
	}

	_, err = s.userRepo.UpdateUser(ctx, userID, updateData)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// ToggleTwoFactor enables or disables two-factor authentication for a user
func (s *UserService) ToggleTwoFactor(ctx context.Context, userID primitive.ObjectID, enable bool) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	// TODO: Implement actual 2FA setup/teardown logic (e.g., generate/verify TOTP secret)

	updateData := bson.M{
		"two_factor_enabled": enable,
		"updated_at":         time.Now(),
	}

	_, err = s.userRepo.UpdateUser(ctx, userID, updateData)
	if err != nil {
		return fmt.Errorf("failed to toggle two-factor authentication: %w", err)
	}

	return nil
}

// DeactivateAccount deactivates a user's account
func (s *UserService) DeactivateAccount(ctx context.Context, userID primitive.ObjectID) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	updateData := bson.M{
		"is_active":  false,
		"updated_at": time.Now(),
	}

	_, err = s.userRepo.UpdateUser(ctx, userID, updateData)
	if err != nil {
		return fmt.Errorf("failed to deactivate account: %w", err)
	}

	// TODO: Invalidate user sessions, log out user, etc.

	return nil
}

// UpdatePrivacySettings updates a user's privacy settings
func (s *UserService) UpdatePrivacySettings(ctx context.Context, userID primitive.ObjectID, req *models.UpdatePrivacySettingsRequest) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	updateFields := bson.M{
		"privacy_settings.last_updated": time.Now(),
	}

	if req.DefaultPostPrivacy != "" {
		updateFields["privacy_settings.default_post_privacy"] = req.DefaultPostPrivacy
	}
	if req.CanSeeMyFriendsList != "" {
		updateFields["privacy_settings.can_see_my_friends_list"] = req.CanSeeMyFriendsList
	}
	if req.CanSendMeFriendRequests != "" {
		updateFields["privacy_settings.can_send_me_friend_requests"] = req.CanSendMeFriendRequests
	}
	if req.CanTagMeInPosts != "" {
		updateFields["privacy_settings.can_tag_me_in_posts"] = req.CanTagMeInPosts
	}

	_, err = s.userRepo.UpdateUser(ctx, userID, updateFields)
	if err != nil {
		return fmt.Errorf("failed to update privacy settings: %w", err)
	}

	return nil
}