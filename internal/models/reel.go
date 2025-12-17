package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Reel struct {
	ID             primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	UserID         primitive.ObjectID     `bson:"user_id" json:"user_id"`
	Author         PostAuthor             `bson:"author,omitempty" json:"author"`
	VideoURL       string                 `bson:"video_url" json:"video_url"`
	ThumbnailURL   string                 `bson:"thumbnail_url" json:"thumbnail_url"`
	Caption        string                 `bson:"caption" json:"caption"`
	Duration       int                    `bson:"duration" json:"duration"` // in seconds
	Views          int64                  `bson:"views" json:"views"`
	Likes          int64                  `bson:"likes" json:"likes"`
	Comments       int64                  `bson:"comments" json:"comments"`
	MsgComments    []Comment              `bson:"msg_comments,omitempty" json:"msg_comments,omitempty"` // Reusing Comment model from feed
	ReactionCounts map[ReactionType]int64 `json:"reaction_counts,omitempty"`
	CreatedAt      time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time              `bson:"updated_at" json:"updated_at"`
}

type CreateReelRequest struct {
	VideoURL     string `json:"video_url" binding:"required"`
	ThumbnailURL string `json:"thumbnail_url" binding:"required"`
	Caption      string `json:"caption"`
	Duration     int    `json:"duration"`
}
