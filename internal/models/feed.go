package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Post represents a single post in the feed
type Post struct {
	ID          primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	UserID      primitive.ObjectID   `bson:"user_id" json:"user_id"`
	Author      PostAuthor           `bson:"-" json:"author"` // Populated from User collection, not stored in Post
	Content     string               `bson:"content" json:"content"`
	MediaType   string               `bson:"media_type,omitempty" json:"media_type,omitempty"` // e.g., "image", "video", "text"
	MediaURL    string               `bson:"media_url,omitempty" json:"media_url,omitempty"`
    Privacy     PrivacySettingType   `bson:"privacy" json:"privacy"` // PUBLIC, FRIENDS, ONLY_ME, CUSTOM
	CustomAudience []primitive.ObjectID `bson:"custom_audience,omitempty" json:"custom_audience,omitempty"` // For CUSTOM privacy
	Comments    []Comment            `bson:"comments" json:"comments"`
	Mentions  []primitive.ObjectID `bson:"mentions,omitempty" json:"mentions,omitempty"`
	ReactionCounts map[ReactionType]int64 `json:"reaction_counts,omitempty"`
	Hashtags    []string             `bson:"hashtags,omitempty,sparse" json:"hashtags,omitempty"`	
	CreatedAt   time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time            `bson:"updated_at" json:"updated_at"`
}

// PostAuthor represents the simplified user information for a post's author
type PostAuthor struct {
	ID       primitive.ObjectID `json:"id"`
	Username string             `json:"username"`
	Avatar   string             `json:"avatar,omitempty"`	
	FullName string             `json:"full_name,omitempty"`
}

// Comment represents a comment on a post
type Comment struct {
	ID        primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	PostID    primitive.ObjectID   `bson:"post_id" json:"post_id"`
	UserID    primitive.ObjectID   `bson:"user_id" json:"user_id"`
	Content   string               `bson:"content" json:"content"`
	Replies   []Reply              `bson:"replies" json:"replies"`
	ReactionCounts map[ReactionType]int64 `json:"reaction_counts,omitempty"`
	Mentions  []primitive.ObjectID `bson:"mentions,omitempty" json:"mentions,omitempty"` // User IDs mentioned in the comment
	CreatedAt time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time            `bson:"updated_at" json:"updated_at"`
}

// Reply represents a reply to a comment
type Reply struct {
	ID        primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	CommentID primitive.ObjectID   `bson:"comment_id" json:"comment_id"`
	Mentions  []primitive.ObjectID `bson:"mentions,omitempty" json:"mentions,omitempty"`
	UserID    primitive.ObjectID   `bson:"user_id" json:"user_id"`
	Content   string               `bson:"content" json:"content"`
	ReactionCounts map[ReactionType]int64 `json:"reaction_counts,omitempty"` // User IDs mentioned in the reply
	CreatedAt time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time            `bson:"updated_at" json:"updated_at"`
}

// ReactionType defines the type of reaction (e.g., Like, Love, Haha)
type ReactionType string

const (
	ReactionLike ReactionType = "LIKE"
	ReactionLove ReactionType = "LOVE"
	ReactionHaha ReactionType = "HAHA"
	ReactionWow  ReactionType = "WOW"
	ReactionSad  ReactionType = "SAD"
	ReactionAngry ReactionType = "ANGRY"
)

// Reaction represents a reaction to a post, comment, or reply
type Reaction struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID       primitive.ObjectID `bson:"user_id" json:"user_id"`
	TargetID     primitive.ObjectID `bson:"target_id" json:"target_id"`         // ID of the post, comment, or reply
	TargetType   string             `bson:"target_type" json:"target_type"`     // "post", "comment", "reply"
	Type         ReactionType       `bson:"type" json:"type"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
}

// DTOs for Feed
type CreatePostRequest struct {
	Content     string               `json:"content" binding:"required"`
	MediaType   string               `json:"media_type,omitempty"`
	MediaURL    string               `json:"media_url,omitempty"`
	Privacy     PrivacySettingType   `json:"privacy" binding:"required"`
	CustomAudience []primitive.ObjectID `json:"custom_audience,omitempty"`
	Mentions    []primitive.ObjectID `json:"mentions,omitempty"`
	Hashtags    []string             `json:"hashtags,omitempty"`
}

type UpdatePostRequest struct {
	Content     string               `json:"content,omitempty"`
	MediaType   string               `json:"media_type,omitempty"`
	MediaURL    string               `json:"media_url,omitempty"`
	Privacy     PrivacySettingType   `json:"privacy,omitempty"`
	CustomAudience []primitive.ObjectID `json:"custom_audience,omitempty"`
	Mentions    []primitive.ObjectID `json:"mentions,omitempty"`
	Hashtags    []string             `json:"hashtags,omitempty"`
}

type CreateCommentRequest struct {
	PostID    primitive.ObjectID   `json:"post_id" binding:"required"`
	Content   string               `json:"content" binding:"required"`
	Mentions  []primitive.ObjectID `json:"mentions,omitempty"`
}

type UpdateCommentRequest struct {
	Content   string               `json:"content" binding:"required"`
	Mentions  []primitive.ObjectID `json:"mentions,omitempty"`
}

type CreateReplyRequest struct {
	CommentID primitive.ObjectID   `json:"comment_id" binding:"required"`
	Content   string               `json:"content" binding:"required"`
	Mentions  []primitive.ObjectID `json:"mentions,omitempty"`
}

type UpdateReplyRequest struct {
	CommentID primitive.ObjectID   `json:"comment_id" binding:"required"`
	Content   string               `json:"content" binding:"required"`
	Mentions  []primitive.ObjectID `json:"mentions,omitempty"`
}

type CreateReactionRequest struct {
	TargetID   primitive.ObjectID `json:"target_id" binding:"required"`
	TargetType string             `json:"target_type" binding:"required"` // "post", "comment", "reply"
	Type       ReactionType       `json:"type" binding:"required"`
}

type FeedResponse struct {
	Posts []Post `json:"posts"`
	Total int64  `json:"total"`
	Page  int64  `json:"page"`
	Limit int64  `json:"limit"`
}