package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
    ID               primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
    Username         string               `bson:"username" json:"username"`
    Email            string               `bson:"email" json:"email"`
    Password         string               `bson:"password" json:"password"`
	Avatar           string               `bson:"avatar" json:"avatar"`
    FullName         string               `bson:"full_name,omitempty" json:"full_name,omitempty"`
    Bio              string               `bson:"bio,omitempty" json:"bio,omitempty"`
    DateOfBirth      *time.Time           `bson:"date_of_birth,omitempty" json:"date_of_birth,omitempty"`
    Gender           string               `bson:"gender,omitempty" json:"gender,omitempty"`
    Location         string               `bson:"location,omitempty" json:"location,omitempty"`
    PhoneNumber      string               `bson:"phone_number,omitempty" json:"phone_number,omitempty"`
    Friends          []primitive.ObjectID `bson:"friends" json:"friends"`
    Blocked          []primitive.ObjectID `bson:"blocked" json:"-"`
    TwoFactorEnabled bool                 `bson:"two_factor_enabled" json:"two_factor_enabled"`
    EmailVerified    bool                 `bson:"email_verified" json:"email_verified"`
    IsActive         bool                 `bson:"is_active" json:"is_active"` // For account deactivation
    LastLogin        *time.Time           `bson:"last_login,omitempty" json:"last_login,omitempty"`
    CreatedAt        time.Time            `bson:"created_at" json:"created_at"`
    UpdatedAt        time.Time            `bson:"updated_at" json:"updated_at"`
    PrivacySettings  UserPrivacySettings  `bson:"privacy_settings" json:"privacy_settings"`
}
type Friendship struct {
    ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    RequesterID primitive.ObjectID `bson:"requester_id" json:"requester_id"`
    ReceiverID  primitive.ObjectID `bson:"receiver_id" json:"receiver_id"`
    Status      FriendshipStatus   `bson:"status" json:"status"` // "pending", "accepted", "rejected"
    CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
    UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

type FriendshipStatus string

const (
	FriendshipStatusPending  FriendshipStatus = "pending"
	FriendshipStatusAccepted FriendshipStatus = "accepted"
	FriendshipStatusRejected FriendshipStatus = "rejected"
	FriendshipStatusBlocked  FriendshipStatus = "blocked"
)


type Group struct {
    ID          primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
    Name        string               `bson:"name" json:"name"`
    CreatorID   primitive.ObjectID   `bson:"creator_id" json:"creator_id"`
    Members     []primitive.ObjectID `bson:"members" json:"members"`
    Admins      []primitive.ObjectID `bson:"admins" json:"admins"`
    CreatedAt   time.Time            `bson:"created_at" json:"created_at"`
    UpdatedAt   time.Time            `bson:"updated_at" json:"updated_at"` 
}

type AuthResponse struct {
	AccessToken  string 			`json:"access_token"`
	RefreshToken string 			`json:"refresh_token"`
	User         SafeUserResponse   `json:"user"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type UserUpdateRequest struct {
	Username        string     `json:"username,omitempty"`
	Email           string     `json:"email,omitempty"`
	CurrentPassword string     `json:"current_password,omitempty"`
	NewPassword     string     `json:"new_password,omitempty"`
	FullName        string     `json:"full_name,omitempty"`
	Bio             string     `json:"bio,omitempty"`
	DateOfBirth     *time.Time `json:"date_of_birth,omitempty"`
	Gender          string     `json:"gender,omitempty"`
	Location        string     `json:"location,omitempty"`
	PhoneNumber     string     `json:"phone_number,omitempty"`
	Avatar          string     `json:"avatar,omitempty"`
}

type UserListResponse struct {
	Users []User `json:"users"`
	Total int64  `json:"total"`
	Page  int64  `json:"page"`
	Limit int64  `json:"limit"`
}

type SafeUserResponse struct {
    ID               primitive.ObjectID   `json:"id"`
    Username         string               `json:"username"`
    Email            string               `json:"email"`
    Avatar           string               `json:"avatar,omitempty"`
    FullName         string               `json:"full_name,omitempty"`
    Bio              string               `json:"bio,omitempty"`
    DateOfBirth      *time.Time           `json:"date_of_birth,omitempty"`
    Gender           string               `json:"gender,omitempty"`
    Location         string               `json:"location,omitempty"`
    PhoneNumber      string               `json:"phone_number,omitempty"`
    Friends          []primitive.ObjectID `json:"friends,omitempty"`
    TwoFactorEnabled bool                 `json:"two_factor_enabled"`
    EmailVerified    bool                 `json:"email_verified"`
    IsActive         bool                 `json:"is_active"`
    LastLogin        *time.Time           `json:"last_login,omitempty"`
    CreatedAt        time.Time            `json:"created_at"`
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
	PrivacySettingPublic          PrivacySettingType = "PUBLIC"
	PrivacySettingFriends         PrivacySettingType = "FRIENDS"
	PrivacySettingOnlyMe          PrivacySettingType = "ONLY_ME"
	PrivacySettingFriendsOfFriends PrivacySettingType = "FRIENDS_OF_FRIENDS"
	PrivacySettingNoOne           PrivacySettingType = "NO_ONE"
	PrivacySettingEveryone        PrivacySettingType = "EVERYONE"
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
	ListID        primitive.ObjectID `bson:"list_id" json:"list_id"`
	MemberUserID  primitive.ObjectID `bson:"member_user_id" json:"member_user_id"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
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
}

func (u *User) ToSafeResponse() SafeUserResponse {
    return SafeUserResponse{
        ID:               u.ID,
        Username:         u.Username,
        Email:            u.Email,
        Avatar:           u.Avatar,
        FullName:         u.FullName,
        Bio:              u.Bio,
        DateOfBirth:      u.DateOfBirth,
        Gender:           u.Gender,
        Location:         u.Location,
        PhoneNumber:      u.PhoneNumber,
        Friends:          u.Friends,
        TwoFactorEnabled: u.TwoFactorEnabled,
        EmailVerified:    u.EmailVerified,
        IsActive:         u.IsActive,
        LastLogin:        u.LastLogin,
        CreatedAt:        u.CreatedAt,
    }
}

