package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
    ID        primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
    Username  string               `bson:"username" json:"username"`
    Email     string               `bson:"email" json:"email"`
    Password  string               `bson:"password" json:"-"`
	Avatar     string              `bson:"avatar" json:"avatar"`
    Friends   []primitive.ObjectID `bson:"friends" json:"friends"`
    Blocked   []primitive.ObjectID `bson:"blocked" json:"-"`
    CreatedAt time.Time            `bson:"created_at" json:"created_at"`
}

type Friendship struct {
    ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    RequesterID primitive.ObjectID `bson:"requester_id" json:"requester_id"`
    ReceiverID  primitive.ObjectID `bson:"receiver_id" json:"receiver_id"`
    Status      string             `bson:"status" json:"status"` // "pending", "accepted", "rejected"
    CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
    UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

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
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         User   `json:"user"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type UserUpdateRequest struct {
	Username        string `json:"username,omitempty"`
	Email           string `json:"email,omitempty"`
	CurrentPassword string `json:"current_password,omitempty"`
	NewPassword     string `json:"new_password,omitempty"`
}

type UserListResponse struct {
	Users []User `json:"users"`
	Total int64  `json:"total"`
	Page  int64  `json:"page"`
	Limit int64  `json:"limit"`
}

const (
	FriendshipStatusPending  = "pending"
	FriendshipStatusAccepted = "accepted"
	FriendshipStatusRejected = "rejected"
	FriendshipStatusBlocked  = "blocked" 
)

const (
	GroupRoleMember = "member"
	GroupRoleAdmin  = "admin"
)