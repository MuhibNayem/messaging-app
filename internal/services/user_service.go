package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/repositories"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo    *repositories.UserRepository
	reelRepo    *repositories.ReelRepository
	redisClient *redis.ClusterClient
	feedService *FeedService
}

func NewUserService(userRepo *repositories.UserRepository, reelRepo *repositories.ReelRepository, redisClient *redis.ClusterClient, feedService *FeedService) *UserService {
	return &UserService{userRepo: userRepo, reelRepo: reelRepo, redisClient: redisClient, feedService: feedService}
}

func (s *UserService) GetUserStatus(ctx context.Context, userID primitive.ObjectID) (map[string]interface{}, error) {
	key := fmt.Sprintf("presence:%s", userID.Hex())
	val, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		// If key not found, assume offline
		if err == redis.Nil {
			return map[string]interface{}{"status": "offline"}, nil
		}
		return nil, err
	}

	var statusData map[string]interface{}
	if err := json.Unmarshal([]byte(val), &statusData); err != nil {
		return nil, err
	}

	return statusData, nil
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

		// Create Post and Add to Album
		go func(newAvatar string) {
			// Create Post
			postReq := &models.CreatePostRequest{
				Content: "Updated their profile picture.",
				Media: []models.MediaItem{{
					Type: "image",
					URL:  newAvatar,
				}},
				Privacy: models.PrivacySettingPublic,
			}
			s.feedService.CreatePost(context.Background(), id, postReq)

			// Add to Album
			album, err := s.feedService.EnsureAlbumExists(context.Background(), id, models.AlbumTypeProfile, "Profile Pictures", true)
			if err == nil && album != nil {
				s.feedService.AddMediaToAlbum(context.Background(), id, album.ID, postReq.Media)
			}
		}(update.Avatar)
	}
	if update.CoverPicture != "" {
		updateData["cover_picture"] = update.CoverPicture

		// Create Post and Add to Album
		go func(newCover string) {
			// Create Post
			postReq := &models.CreatePostRequest{
				Content: "Updated their cover photo.",
				Media: []models.MediaItem{{
					Type: "image",
					URL:  newCover,
				}},
				Privacy: models.PrivacySettingPublic,
			}
			s.feedService.CreatePost(context.Background(), id, postReq)

			// Add to Album
			album, err := s.feedService.EnsureAlbumExists(context.Background(), id, models.AlbumTypeCover, "Cover Photos", true)
			if err == nil && album != nil {
				s.feedService.AddMediaToAlbum(context.Background(), id, album.ID, postReq.Media)
			}
		}(update.CoverPicture)
	}
	if update.IsEncryptionEnabled != nil {
		updateData["is_encryption_enabled"] = *update.IsEncryptionEnabled
	}

	updatedUser, err := s.userRepo.UpdateUser(ctx, id, updateData)
	if err != nil {
		return nil, err
	}

	// Clear password before returning
	updatedUser.Password = ""

	// Sync author info to Reels (async or sync? Sync for now to ensure consistency)
	if update.Username != "" || update.Avatar != "" || update.FullName != "" {
		go func() {
			author := models.PostAuthor{
				ID:       updatedUser.ID.Hex(),
				Username: updatedUser.Username,
				Avatar:   updatedUser.Avatar,
				FullName: updatedUser.FullName,
			}
			// We use a background context or a new one
			if err := s.reelRepo.UpdateAuthorInfo(context.Background(), id, author); err != nil {
				log.Printf("Failed to sync author info to reels: %v", err)
			}
		}()
	}

	return updatedUser, nil
}

// UpdateNotificationSettings updates a user's notification settings
func (s *UserService) UpdateNotificationSettings(ctx context.Context, userID primitive.ObjectID, req *models.UpdateNotificationSettingsRequest) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	updateFields := bson.M{}

	if req.EmailNotifications != nil {
		updateFields["notification_settings.email_notifications"] = *req.EmailNotifications
	}
	if req.PushNotifications != nil {
		updateFields["notification_settings.push_notifications"] = *req.PushNotifications
	}
	if req.NotifyOnFriendRequest != nil {
		updateFields["notification_settings.notify_on_friend_request"] = *req.NotifyOnFriendRequest
	}
	if req.NotifyOnComment != nil {
		updateFields["notification_settings.notify_on_comment"] = *req.NotifyOnComment
	}
	if req.NotifyOnLike != nil {
		updateFields["notification_settings.notify_on_like"] = *req.NotifyOnLike
	}
	if req.NotifyOnTag != nil {
		updateFields["notification_settings.notify_on_tag"] = *req.NotifyOnTag
	}
	if req.NotifyOnMessage != nil {
		updateFields["notification_settings.notify_on_message"] = *req.NotifyOnMessage
	}

	if len(updateFields) == 0 {
		return nil // No updates
	}

	_, err = s.userRepo.UpdateUser(ctx, userID, updateFields)
	if err != nil {
		return fmt.Errorf("failed to update notification settings: %w", err)
	}

	return nil
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

// UpdatePublicKey updates a user's E2EE public key and encrypted private key backup
func (s *UserService) UpdatePublicKey(ctx context.Context, userID primitive.ObjectID, publicKey, encryptedPrivateKey, iv, salt string) error {
	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("user not found")
	}

	updateData := bson.M{
		"public_key":            publicKey,
		"encrypted_private_key": encryptedPrivateKey,
		"key_backup_iv":         iv,
		"key_backup_salt":       salt,
		"updated_at":            time.Now(),
	}

	_, err = s.userRepo.UpdateUser(ctx, userID, updateData)
	if err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	return nil
}

// GetUsersPresence retrieves the presence status for a list of user IDs
func (s *UserService) GetUsersPresence(ctx context.Context, userIDs []primitive.ObjectID) (map[string]map[string]interface{}, error) {
	presenceMap := make(map[string]map[string]interface{})

	keys := make([]string, len(userIDs))
	for i, userID := range userIDs {
		keys[i] = fmt.Sprintf("presence:%s", userID.Hex())
	}

	// MGET all presence keys from Redis
	vals, err := s.redisClient.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get presence from Redis: %w", err)
	}

	for i, userID := range userIDs {
		var statusData map[string]interface{}
		val := vals[i]

		if val == nil {
			// Key not found, assume offline
			statusData = map[string]interface{}{"status": "offline", "last_seen": time.Now().Unix()}
		} else {
			if err := json.Unmarshal([]byte(val.(string)), &statusData); err != nil {
				// Error unmarshaling, assume offline and log error
				log.Printf("Error unmarshaling presence data for user %s: %v", userID.Hex(), err)
				statusData = map[string]interface{}{"status": "offline", "last_seen": time.Now().Unix()}
			}
		}
		presenceMap[userID.Hex()] = statusData
	}

	return presenceMap, nil
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
