package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AlbumType string

const (
	AlbumTypeCustom   AlbumType = "custom"
	AlbumTypeProfile  AlbumType = "profile"
	AlbumTypeCover    AlbumType = "cover"
	AlbumTypeTimeline AlbumType = "timeline" // Virtual, derived from posts not in other albums
)

type Album struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID      primitive.ObjectID `bson:"user_id" json:"user_id"`
	Name        string             `bson:"name" json:"name"`
	Description string             `bson:"description,omitempty" json:"description"`
	Type        AlbumType          `bson:"type" json:"type"`
	CoverURL    string             `bson:"cover_url,omitempty" json:"cover_url"`
	// Media field removed in favor of separate album_media collection
	PostIDs   []primitive.ObjectID `bson:"post_ids" json:"post_ids"`
	Privacy   PrivacySettingType   `bson:"privacy" json:"privacy"`
	CreatedAt time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time            `bson:"updated_at" json:"updated_at"`
}

type AlbumMedia struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AlbumID   primitive.ObjectID `bson:"album_id" json:"album_id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	URL       string             `bson:"url" json:"url"`
	Type      string             `bson:"type" json:"type"` // "image", "video"
	Caption   string             `bson:"caption,omitempty" json:"caption,omitempty"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

type CreateAlbumRequest struct {
	Name        string             `json:"name" binding:"required"`
	Description string             `json:"description,omitempty"`
	Privacy     PrivacySettingType `json:"privacy"`
}

type UpdateAlbumRequest struct {
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	CoverURL    string             `json:"cover_url,omitempty"`
	Privacy     PrivacySettingType `json:"privacy,omitempty"`
}

type AddMediaToAlbumRequest struct {
	Media []MediaItem `json:"media" binding:"required"`
}
