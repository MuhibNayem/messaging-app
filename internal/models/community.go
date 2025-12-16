package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CommunityPrivacy string

const (
	CommunityPrivacyPublic CommunityPrivacy = "public"
	CommunityPrivacyClosed CommunityPrivacy = "closed" // Private but searchable, requires approval to join
	CommunityPrivacySecret CommunityPrivacy = "secret" // Not searchable, invite only
)

type Community struct {
	ID             primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	Name           string               `bson:"name" json:"name"`
	Description    string               `bson:"description" json:"description"`
	Slug           string               `bson:"slug" json:"slug"` // Unique URL-friendly identifier
	Category       string               `bson:"category" json:"category"`
	Avatar         string               `bson:"avatar" json:"avatar"`
	CoverImage     string               `bson:"cover_image" json:"cover_image"`
	Privacy        CommunityPrivacy     `bson:"privacy" json:"privacy"`
	CreatorID      primitive.ObjectID   `bson:"creator_id" json:"creator_id"`
	Members        []primitive.ObjectID `bson:"members" json:"members"`
	Admins         []primitive.ObjectID `bson:"admins" json:"admins"`
	PendingMembers []primitive.ObjectID `bson:"pending_members" json:"pending_members"`
	BannedUsers    []primitive.ObjectID `bson:"banned_users" json:"banned_users"`
	Settings       CommunitySettings    `bson:"settings" json:"settings"`
	Stats          CommunityStats       `bson:"stats" json:"stats"`
	CreatedAt      time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time            `bson:"updated_at" json:"updated_at"`
}

type CommunitySettings struct {
	RequirePostApproval bool `bson:"require_post_approval" json:"require_post_approval"`
	RequireJoinApproval bool `bson:"require_join_approval" json:"require_join_approval"`
}

type CommunityStats struct {
	MemberCount int64 `bson:"member_count" json:"member_count"`
	PostCount   int64 `bson:"post_count" json:"post_count"`
}

// DTOs for Community

type CreateCommunityRequest struct {
	Name                string           `json:"name" binding:"required,min=3,max=50"`
	Description         string           `json:"description" binding:"required,max=500"`
	Category            string           `json:"category" binding:"required"`
	Privacy             CommunityPrivacy `json:"privacy" binding:"required,oneof=public closed secret"`
	RequirePostApproval bool             `json:"require_post_approval"`
	RequireJoinApproval bool             `json:"require_join_approval"`
}

type UpdateCommunityRequest struct {
	Name                string           `json:"name,omitempty" binding:"omitempty,min=3,max=50"`
	Description         string           `json:"description,omitempty" binding:"omitempty,max=500"`
	Category            string           `json:"category,omitempty"`
	Avatar              string           `json:"avatar,omitempty"`
	CoverImage          string           `json:"cover_image,omitempty"`
	Privacy             CommunityPrivacy `json:"privacy,omitempty" binding:"omitempty,oneof=public closed secret"`
	RequirePostApproval *bool            `json:"require_post_approval,omitempty"`
	RequireJoinApproval *bool            `json:"require_join_approval,omitempty"`
}

type CommunityResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Slug        string            `json:"slug"`
	Category    string            `json:"category"`
	Avatar      string            `json:"avatar"`
	CoverImage  string            `json:"cover_image"`
	Privacy     CommunityPrivacy  `json:"privacy"`
	Settings    CommunitySettings `json:"settings"`
	Stats       CommunityStats    `json:"stats"`
	IsMember    bool              `json:"is_member"`
	IsAdmin     bool              `json:"is_admin"`
	IsPending   bool              `json:"is_pending"`
	CreatedAt   time.Time         `json:"created_at"`
}
