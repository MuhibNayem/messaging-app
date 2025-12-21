package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type PostStatus string

const (
	PostStatusActive   PostStatus = "active"
	PostStatusPending  PostStatus = "pending"
	PostStatusDeclined PostStatus = "declined"
)

type MediaItem struct {
	URL  string `bson:"url" json:"url"`
	Type string `bson:"type" json:"type"` // "image", "video"
}

// Post represents a single post in the feed
type Post struct {
	ID                     primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	UserID                 primitive.ObjectID     `bson:"user_id" json:"user_id"`
	Author                 PostAuthor             `bson:"author,omitempty" json:"author"` // Populated from User collection, not stored in Post
	Content                string                 `bson:"content" json:"content"`
	Media                  []MediaItem            `bson:"media,omitempty" json:"media,omitempty"`
	Location               string                 `bson:"location,omitempty" json:"location,omitempty"`
	Privacy                PrivacySettingType     `bson:"privacy" json:"privacy"`                                     // PUBLIC, FRIENDS, ONLY_ME, CUSTOM
	Status                 PostStatus             `bson:"status" json:"status"`                                       // New: active, pending, declined
	CommunityID            *primitive.ObjectID    `bson:"community_id,omitempty" json:"community_id,omitempty"`       // If post belongs to a community
	CustomAudience         []primitive.ObjectID   `bson:"custom_audience,omitempty" json:"custom_audience,omitempty"` // For CUSTOM privacy
	CommentIDs             []primitive.ObjectID   `bson:"comment_ids" json:"-"`                                       // Stored as IDs in DB, not directly exposed in JSON
	Comments               []Comment              `bson:"comments,omitempty" json:"comments"`                         // Populated full Comment objects, not stored in DB
	Mentions               []primitive.ObjectID   `bson:"mentions,omitempty" json:"mentions,omitempty"`
	MentionedUsers         []PostAuthor           `bson:"mentioned_users,omitempty" json:"mentioned_users,omitempty"`
	SpecificReactionCounts map[ReactionType]int64 `json:"specific_reaction_counts,omitempty"`
	Hashtags               []string               `bson:"hashtags,omitempty,sparse" json:"hashtags,omitempty"`
	TotalReactions         int64                  `bson:"total_reactions" json:"total_reactions"` // Denormalized count
	TotalComments          int64                  `bson:"total_comments" json:"total_comments"`   // Denormalized count
	CreatedAt              time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt              time.Time              `bson:"updated_at" json:"updated_at"`
}

// PostAuthor represents the simplified user information for a post's author
type PostAuthor struct {
	ID       string `bson:"id" json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar,omitempty"`
	FullName string `json:"full_name,omitempty"`
}

// Comment represents a comment on a post
type Comment struct {
	ID             primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	PostID         primitive.ObjectID     `bson:"post_id,omitempty" json:"post_id,omitempty"` // Optional if for Reel
	ReelID         *primitive.ObjectID    `bson:"reel_id,omitempty" json:"reel_id,omitempty"` // Optional if for Post
	UserID         primitive.ObjectID     `bson:"user_id" json:"user_id"`
	Author         PostAuthor             `bson:"author,omitempty" json:"author"`
	Content        string                 `bson:"content" json:"content"`
	MediaType      string                 `bson:"media_type,omitempty" json:"media_type,omitempty"`
	MediaURL       string                 `bson:"media_url,omitempty" json:"media_url,omitempty"`
	Replies        []Reply                `bson:"replies,omitempty" json:"replies"` // Populated full Reply objects, not stored in DB
	Reactions      []Reaction             `bson:"reactions,omitempty" json:"reactions,omitempty"`
	ReactionCounts map[ReactionType]int64 `json:"reaction_counts,omitempty"`
	Mentions       []primitive.ObjectID   `bson:"mentions,omitempty" json:"mentions,omitempty"` // User IDs mentioned in the comment
	CreatedAt      time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time              `bson:"updated_at" json:"updated_at"`
}

// Reply represents a reply to a comment
type Reply struct {
	ID             primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	CommentID      primitive.ObjectID     `bson:"comment_id" json:"comment_id"`
	ParentReplyID  *primitive.ObjectID    `bson:"parent_reply_id,omitempty" json:"parent_reply_id,omitempty"`
	UserID         primitive.ObjectID     `bson:"user_id" json:"user_id"`
	Author         PostAuthor             `bson:"author,omitempty" json:"author"`
	Mentions       []primitive.ObjectID   `bson:"mentions,omitempty" json:"mentions,omitempty"`
	Content        string                 `bson:"content" json:"content"`
	MediaType      string                 `bson:"media_type,omitempty" json:"media_type,omitempty"`
	MediaURL       string                 `bson:"media_url,omitempty" json:"media_url,omitempty"`
	ReactionCounts map[ReactionType]int64 `json:"reaction_counts,omitempty"` // User IDs mentioned in the reply
	CreatedAt      time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time              `bson:"updated_at" json:"updated_at"`
}

// ReactionType defines the type of reaction (e.g., Like, Love, Haha)
type ReactionType string

const (
	ReactionLike  ReactionType = "LIKE"
	ReactionLove  ReactionType = "LOVE"
	ReactionHaha  ReactionType = "HAHA"
	ReactionWow   ReactionType = "WOW"
	ReactionSad   ReactionType = "SAD"
	ReactionAngry ReactionType = "ANGRY"
)

// Reaction represents a reaction to a post, comment, or reply
type Reaction struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID `bson:"user_id" json:"user_id"`
	TargetID   primitive.ObjectID `bson:"target_id" json:"target_id"`     // ID of the post, comment, or reply
	TargetType string             `bson:"target_type" json:"target_type"` // "post", "comment", "reply"
	Type       ReactionType       `bson:"type" json:"type"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}

// DTOs for Feed
type CreatePostRequest struct {
	Content        string               `json:"content" form:"content" binding:"required"`
	Media          []MediaItem          `json:"media,omitempty"` // Media is handled separately for multipart
	Location       string               `json:"location,omitempty" form:"location"`
	Privacy        PrivacySettingType   `json:"privacy" form:"privacy" binding:"required"`
	CommunityID    string               `json:"community_id,omitempty" form:"community_id"` // Optional community ID
	CustomAudience []primitive.ObjectID `json:"custom_audience,omitempty" form:"custom_audience"`
	Mentions       []primitive.ObjectID `json:"mentions,omitempty" form:"mentions"`
	Hashtags       []string             `json:"hashtags,omitempty" form:"hashtags"`
}

type UpdatePostRequest struct {
	Content        string               `json:"content,omitempty"`
	Media          []MediaItem          `json:"media,omitempty"`
	Location       string               `json:"location,omitempty"`
	Privacy        PrivacySettingType   `json:"privacy,omitempty"`
	CustomAudience []primitive.ObjectID `json:"custom_audience,omitempty"`
	Mentions       []primitive.ObjectID `json:"mentions,omitempty"`
	Hashtags       []string             `json:"hashtags,omitempty"`
}

type CreateCommentRequest struct {
	PostID    *primitive.ObjectID  `json:"post_id,omitempty"`
	ReelID    *primitive.ObjectID  `json:"reel_id,omitempty"`
	Content   string               `json:"content" binding:"required"`
	MediaType string               `json:"media_type,omitempty"`
	MediaURL  string               `json:"media_url,omitempty"`
	Mentions  []primitive.ObjectID `json:"mentions,omitempty"`
}

type UpdateCommentRequest struct {
	Content  string               `json:"content" binding:"required"`
	Mentions []primitive.ObjectID `json:"mentions,omitempty"`
}

type CreateReplyRequest struct {
	CommentID     primitive.ObjectID   `json:"comment_id" binding:"required"`
	ParentReplyID *primitive.ObjectID  `json:"parent_reply_id,omitempty"`
	Content       string               `json:"content" binding:"required"`
	MediaType     string               `json:"media_type,omitempty"`
	MediaURL      string               `json:"media_url,omitempty"`
	Mentions      []primitive.ObjectID `json:"mentions,omitempty"`
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
