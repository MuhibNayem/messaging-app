package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Story struct {
	ID             primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	UserID         primitive.ObjectID   `bson:"user_id" json:"user_id"`
	Author         PostAuthor           `bson:"author,omitempty" json:"author"` // Denormalized author info
	MediaURL       string               `bson:"media_url" json:"media_url"`
	MediaType      string               `bson:"media_type" json:"media_type"` // "image" or "video"
	Privacy        PrivacySettingType   `bson:"privacy" json:"privacy"`
	AllowedViewers []primitive.ObjectID `bson:"allowed_viewers,omitempty" json:"allowed_viewers,omitempty"` // For CUSTOM
	BlockedViewers []primitive.ObjectID `bson:"blocked_viewers,omitempty" json:"blocked_viewers,omitempty"` // For FRIENDS_EXCEPT
	Viewers        []primitive.ObjectID `bson:"viewers,omitempty" json:"viewers,omitempty"`
	CreatedAt      time.Time            `bson:"created_at" json:"created_at"`
	ExpiresAt      time.Time            `bson:"expires_at" json:"expires_at"`
}

type CreateStoryRequest struct {
	MediaURL       string               `json:"media_url" binding:"required"`
	MediaType      string               `json:"media_type" binding:"required"`
	Privacy        PrivacySettingType   `json:"privacy"` // Defaults to FRIENDS if empty
	AllowedViewers []primitive.ObjectID `json:"allowed_viewers,omitempty"`
	BlockedViewers []primitive.ObjectID `json:"blocked_viewers,omitempty"`
}
