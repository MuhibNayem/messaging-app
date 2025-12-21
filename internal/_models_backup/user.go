package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID                   primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	Username             string               `bson:"username" json:"username"`
	Email                string               `bson:"email" json:"email"`
	Password             string               `bson:"password" json:"password"`
	Avatar               string               `bson:"avatar" json:"avatar"`
	CoverPicture         string               `bson:"cover_picture,omitempty" json:"cover_picture,omitempty"`
	FullName             string               `bson:"full_name,omitempty" json:"full_name,omitempty"`
	Bio                  string               `bson:"bio,omitempty" json:"bio,omitempty"`
	DateOfBirth          *time.Time           `bson:"date_of_birth,omitempty" json:"date_of_birth,omitempty"`
	Gender               string               `bson:"gender,omitempty" json:"gender,omitempty"`
	Location             string               `bson:"location,omitempty" json:"location,omitempty"`
	PhoneNumber          string               `bson:"phone_number,omitempty" json:"phone_number,omitempty"`
	Friends              []primitive.ObjectID `bson:"friends" json:"friends"`
	Blocked              []primitive.ObjectID `bson:"blocked" json:"-"`
	TwoFactorEnabled     bool                 `bson:"two_factor_enabled" json:"two_factor_enabled"`
	EmailVerified        bool                 `bson:"email_verified" json:"email_verified"`
	IsActive             bool                 `bson:"is_active" json:"is_active"` // For account deactivation
	LastLogin            *time.Time           `bson:"last_login,omitempty" json:"last_login,omitempty"`
	CreatedAt            time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt            time.Time            `bson:"updated_at" json:"updated_at"`
	PrivacySettings      UserPrivacySettings  `bson:"privacy_settings" json:"privacy_settings"`
	NotificationSettings NotificationSettings `bson:"notification_settings" json:"notification_settings"`
	PublicKey            string               `bson:"public_key,omitempty" json:"public_key,omitempty"`                       // E2EE Public Key
	EncryptedPrivateKey  string               `bson:"encrypted_private_key,omitempty" json:"encrypted_private_key,omitempty"` // E2EE Backup
	KeyBackupIV          string               `bson:"key_backup_iv,omitempty" json:"key_backup_iv,omitempty"`                 // E2EE Backup
	KeyBackupSalt        string               `bson:"key_backup_salt,omitempty" json:"key_backup_salt,omitempty"`             // E2EE Backup
	IsEncryptionEnabled  bool                 `bson:"is_encryption_enabled" json:"is_encryption_enabled"`                     // Persistent Toggle
}

type NotificationSettings struct {
	EmailNotifications    bool `bson:"email_notifications" json:"email_notifications"`
	PushNotifications     bool `bson:"push_notifications" json:"push_notifications"`
	NotifyOnFriendRequest bool `bson:"notify_on_friend_request" json:"notify_on_friend_request"`
	NotifyOnComment       bool `bson:"notify_on_comment" json:"notify_on_comment"`
	NotifyOnLike          bool `bson:"notify_on_like" json:"notify_on_like"`
	NotifyOnTag           bool `bson:"notify_on_tag" json:"notify_on_tag"`
	NotifyOnMessage       bool `bson:"notify_on_message" json:"notify_on_message"`
	NotifyOnBirthday      bool `bson:"notify_on_birthday" json:"notify_on_birthday"`
	NotifyOnEventInvite   bool `bson:"notify_on_event_invite" json:"notify_on_event_invite"`
}

type Friendship struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	RequesterID primitive.ObjectID `bson:"requester_id" json:"requester_id"`
	ReceiverID  primitive.ObjectID `bson:"receiver_id" json:"receiver_id"`
	Status      FriendshipStatus   `bson:"status" json:"status"` // "pending", "accepted", "rejected"
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

// PopulatedFriendship is used for API responses where user details are embedded.
type PopulatedFriendship struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	RequesterID   primitive.ObjectID `bson:"requester_id" json:"requester_id"`
	ReceiverID    primitive.ObjectID `bson:"receiver_id" json:"receiver_id"`
	RequesterInfo SafeUserResponse   `bson:"requester_info" json:"requester_info"`
	ReceiverInfo  SafeUserResponse   `bson:"receiver_info" json:"receiver_info"`
	Status        FriendshipStatus   `bson:"status" json:"status"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time          `bson:"updated_at" json:"updated_at"`
}

type FriendshipStatus string

const (
	FriendshipStatusPending  FriendshipStatus = "pending"
	FriendshipStatusAccepted FriendshipStatus = "accepted"
	FriendshipStatusRejected FriendshipStatus = "rejected"
	FriendshipStatusBlocked  FriendshipStatus = "blocked"
)

type Group struct {
	ID             primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	Name           string               `bson:"name" json:"name"`
	Avatar         string               `bson:"avatar,omitempty" json:"avatar,omitempty"`
	CreatorID      primitive.ObjectID   `bson:"creator_id" json:"creator_id"`
	Members        []primitive.ObjectID `bson:"members" json:"members"`
	PendingMembers []primitive.ObjectID `bson:"pending_members" json:"pending_members"`
	Admins         []primitive.ObjectID `bson:"admins" json:"admins"`
	Settings       GroupSettings        `bson:"settings" json:"settings"`
	CreatedAt      time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time            `bson:"updated_at" json:"updated_at"`
}

type GroupSettings struct {
	RequiresApproval bool `bson:"requires_approval" json:"requires_approval"`
}

type AuthResponse struct {
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	User         SafeUserResponse `json:"user"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type UserShortResponse struct {
	ID        primitive.ObjectID `bson:"_id" json:"id"`
	Username  string             `bson:"username" json:"username"`
	Email     string             `bson:"email" json:"email"`
	FullName  string             `bson:"full_name,omitempty" json:"full_name,omitempty"`
	Avatar    string             `bson:"avatar,omitempty" json:"avatar,omitempty"`
	PublicKey string             `bson:"public_key,omitempty" json:"public_key,omitempty"`
}

type GroupResponse struct {
	ID             primitive.ObjectID  `json:"id"`
	Name           string              `json:"name"`
	Avatar         string              `json:"avatar,omitempty"`
	Creator        UserShortResponse   `json:"creator"`
	Members        []UserShortResponse `json:"members"`
	PendingMembers []UserShortResponse `json:"pending_members"`
	Admins         []UserShortResponse `json:"admins"`
	Settings       GroupSettings       `json:"settings"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}
type UserUpdateRequest struct {
	Username            string     `json:"username,omitempty"`
	Email               string     `json:"email,omitempty"`
	CurrentPassword     string     `json:"current_password,omitempty"`
	NewPassword         string     `json:"new_password,omitempty"`
	FullName            string     `json:"full_name,omitempty"`
	Bio                 string     `json:"bio,omitempty"`
	DateOfBirth         *time.Time `json:"date_of_birth,omitempty"`
	Gender              string     `json:"gender,omitempty"`
	Location            string     `json:"location,omitempty"`
	PhoneNumber         string     `json:"phone_number,omitempty"`
	Avatar              string     `json:"avatar,omitempty"`
	CoverPicture        string     `json:"cover_picture,omitempty"`
	IsEncryptionEnabled *bool      `json:"is_encryption_enabled,omitempty"` // Pointer to allow false
}

type UserListResponse struct {
	Users []User `json:"users"`
	Total int64  `json:"total"`
	Page  int64  `json:"page"`
	Limit int64  `json:"limit"`
}

type SafeUserResponse struct {
	ID                  primitive.ObjectID   `bson:"_id" json:"id"`
	Username            string               `json:"username"`
	Email               string               `json:"email"`
	Avatar              string               `json:"avatar,omitempty"`
	FullName            string               `json:"full_name,omitempty"`
	Bio                 string               `json:"bio,omitempty"`
	DateOfBirth         *time.Time           `json:"date_of_birth,omitempty"`
	Gender              string               `json:"gender,omitempty"`
	Location            string               `json:"location,omitempty"`
	PhoneNumber         string               `json:"phone_number,omitempty"`
	Friends             []primitive.ObjectID `json:"friends,omitempty"`
	TwoFactorEnabled    bool                 `json:"two_factor_enabled"`
	EmailVerified       bool                 `json:"email_verified"`
	IsActive            bool                 `json:"is_active"`
	LastLogin           *time.Time           `json:"last_login,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	PublicKey           string               `json:"public_key,omitempty"`
	EncryptedPrivateKey string               `json:"encrypted_private_key,omitempty"`
	KeyBackupIV         string               `json:"key_backup_iv,omitempty"`
	KeyBackupSalt       string               `json:"key_backup_salt,omitempty"`
	IsEncryptionEnabled bool                 `json:"is_encryption_enabled"`
}

type UserPrivacySettings struct {
	UserID                  primitive.ObjectID `bson:"user_id" json:"user_id"`
	DefaultPostPrivacy      PrivacySettingType `bson:"default_post_privacy" json:"default_post_privacy"`
	CanSeeMyFriendsList     PrivacySettingType `bson:"can_see_my_friends_list" json:"can_see_my_friends_list"`
	CanSendMeFriendRequests PrivacySettingType `bson:"can_send_me_friend_requests" json:"can_send_me_friend_requests"`
	CanTagMeInPosts         PrivacySettingType `bson:"can_tag_me_in_posts" json:"can_tag_me_in_posts"`
	LastUpdated             time.Time          `bson:"last_updated" json:"last_updated"`
}

type PrivacySettingType string

const (
	PrivacySettingPublic           PrivacySettingType = "PUBLIC"
	PrivacySettingFriends          PrivacySettingType = "FRIENDS"
	PrivacySettingOnlyMe           PrivacySettingType = "ONLY_ME"
	PrivacySettingFriendsOfFriends PrivacySettingType = "FRIENDS_OF_FRIENDS"
	PrivacySettingNoOne            PrivacySettingType = "NO_ONE"
	PrivacySettingEveryone         PrivacySettingType = "EVERYONE"
	PrivacySettingCustom           PrivacySettingType = "CUSTOM"         // Specific Friends
	PrivacySettingFriendsExcept    PrivacySettingType = "FRIENDS_EXCEPT" // Friends except specific ones
)

type CustomPrivacyList struct {
	ID        primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID   `bson:"user_id" json:"user_id"`
	Name      string               `bson:"name" json:"name"`
	Members   []primitive.ObjectID `bson:"members" json:"members"`
	CreatedAt time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time            `bson:"updated_at" json:"updated_at"`
}

type CustomPrivacyListMember struct {
	ListID       primitive.ObjectID `bson:"list_id" json:"list_id"`
	MemberUserID primitive.ObjectID `bson:"member_user_id" json:"member_user_id"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
}

// DTOs for Privacy Settings
type UpdatePrivacySettingsRequest struct {
	DefaultPostPrivacy      PrivacySettingType `json:"default_post_privacy,omitempty"`
	CanSeeMyFriendsList     PrivacySettingType `json:"can_see_my_friends_list,omitempty"`
	CanSendMeFriendRequests PrivacySettingType `json:"can_send_me_friend_requests,omitempty"`
	CanTagMeInPosts         PrivacySettingType `json:"can_tag_me_in_posts,omitempty"`
}

type CreateCustomPrivacyListRequest struct {
	Name    string               `json:"name" binding:"required"`
	Members []primitive.ObjectID `json:"members,omitempty"`
}

type UpdateCustomPrivacyListRequest struct {
	Name    string               `json:"name,omitempty"`
	Members []primitive.ObjectID `json:"members,omitempty"`
}

type AddRemoveCustomPrivacyListMemberRequest struct {
	UserID primitive.ObjectID `json:"user_id" binding:"required"`
}

type UpdateNotificationSettingsRequest struct {
	EmailNotifications    *bool `json:"email_notifications,omitempty"`
	PushNotifications     *bool `json:"push_notifications,omitempty"`
	NotifyOnFriendRequest *bool `json:"notify_on_friend_request,omitempty"`
	NotifyOnComment       *bool `json:"notify_on_comment,omitempty"`
	NotifyOnLike          *bool `json:"notify_on_like,omitempty"`
	NotifyOnTag           *bool `json:"notify_on_tag,omitempty"`
	NotifyOnMessage       *bool `json:"notify_on_message,omitempty"`
	NotifyOnBirthday      *bool `json:"notify_on_birthday,omitempty"`
	NotifyOnEventInvite   *bool `json:"notify_on_event_invite,omitempty"`
}

// DTOs for Account Settings
type UpdateUserProfileRequest struct {
	FullName    string     `json:"full_name,omitempty"`
	Bio         string     `json:"bio,omitempty"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty"`
	Gender      string     `json:"gender,omitempty"`
	Location    string     `json:"location,omitempty"`
	PhoneNumber string     `json:"phone_number,omitempty"`
	Avatar      string     `json:"avatar,omitempty"`
}

type UpdateEmailRequest struct {
	NewEmail string `json:"new_email" binding:"required,email"`
}

type UpdatePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

type ToggleTwoFactorRequest struct {
	Enabled bool `json:"enabled" binding:"required"`
}

func (u *User) SetDefaultPrivacySettings() {
	u.PrivacySettings = UserPrivacySettings{
		DefaultPostPrivacy:      PrivacySettingPublic,
		CanSeeMyFriendsList:     PrivacySettingFriends,
		CanSendMeFriendRequests: PrivacySettingEveryone,
		CanTagMeInPosts:         PrivacySettingEveryone,
		LastUpdated:             time.Now(),
	}
	u.IsEncryptionEnabled = false // Default to disabled
}

func (u *User) ToSafeResponse() SafeUserResponse {
	return SafeUserResponse{
		ID:                  u.ID,
		Username:            u.Username,
		Email:               u.Email,
		Avatar:              u.Avatar,
		FullName:            u.FullName,
		Bio:                 u.Bio,
		DateOfBirth:         u.DateOfBirth,
		Gender:              u.Gender,
		Location:            u.Location,
		PhoneNumber:         u.PhoneNumber,
		Friends:             u.Friends,
		TwoFactorEnabled:    u.TwoFactorEnabled,
		EmailVerified:       u.EmailVerified,
		IsActive:            u.IsActive,
		LastLogin:           u.LastLogin,
		KeyBackupSalt:       u.KeyBackupSalt,
		IsEncryptionEnabled: u.IsEncryptionEnabled,
	}
}
