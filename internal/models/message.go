package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Message struct {
	ID          primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	SenderID    primitive.ObjectID   `bson:"sender_id" json:"sender_id"`
	SenderName  string               `bson:"sender_name,omitempty" json:"sender_name,omitempty"`
	ReceiverID  primitive.ObjectID   `bson:"receiver_id,omitempty" json:"receiver_id,omitempty"`
	GroupID     primitive.ObjectID   `bson:"group_id,omitempty" json:"group_id,omitempty"`
	GroupName   string               `bson:"group_name,omitempty" json:"group_name,omitempty"`
	Content     string               `bson:"content,omitempty" json:"content,omitempty"` 
	ContentType string               `bson:"content_type" json:"content_type"`
	MediaURLs   []string             `bson:"media_urls,omitempty" json:"media_urls,omitempty"`
	SeenBy      []primitive.ObjectID `bson:"seen_by" json:"seen_by"`
	IsDeleted       bool       `bson:"is_deleted" json:"is_deleted"`
    DeletedAt      *time.Time `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
    OriginalContent string     `bson:"original_content,omitempty" json:"-"`
	CreatedAt   time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time            `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type TypingEvent struct {
    ConversationID string `json:"conversation_id"` // group_id or user_id
    UserID        string `json:"user_id"`
    IsTyping      bool   `json:"is_typing"`
    Timestamp     int64  `json:"timestamp"`
}

type MessageQuery struct {
	GroupID    string `form:"group_id"`
	ConversationID    string `form:"group_id"`
	SenderID    string `form:"sender_id"`
	ReceiverID string `form:"receiver_id"`
	Page       int    `form:"page,default=1"`
	Limit      int    `form:"limit,default=50"`
	Before     string `form:"before"` 
}

type MessageRequest struct {
	ReceiverID  string   `json:"receiver_id,omitempty"`
	SenderID    string   `json:"sender_id,omitempty"`
	GroupID     string   `json:"group_id,omitempty"`
	Content     string   `json:"content,omitempty"`
	ContentType string   `json:"content_type"`
	MediaURLs   []string `json:"media_urls,omitempty"` 
}

type MessageResponse struct {
	Messages []Message `json:"messages"`
	Total    int64     `json:"total"`
	Page     int64     `json:"page"`
	Limit    int64     `json:"limit"`
	HasMore  bool      `json:"has_more"` 
}

// Helper struct for message status updates
type MessageStatusUpdate struct {
	MessageID primitive.ObjectID `json:"message_id"`
	UserID    primitive.ObjectID `json:"user_id"`
	Action    string             `json:"action"` // "seen", "delivered", etc.
}

type ErrorResponse struct {
    Error string `json:"error"`
}

type SuccessResponse struct {
    Success bool `json:"success"`
}

type UnreadCountResponse struct {
    Count int64 `json:"count"`
}

// Content type constants
const (
    ContentTypeText      = "text"
    ContentTypeImage     = "image"
    ContentTypeVideo     = "video"
    ContentTypeFile      = "file"
    ContentTypeAudio     = "audio"
    ContentTypeTextImage = "text_image"
    ContentTypeTextVideo = "text_video"
    ContentTypeTextFile  = "text_file"
    ContentTypeMultiple  = "multiple"
    ContentTypeDeleted   = "deleted"
)

var ValidContentTypes = map[string]bool{
    ContentTypeText:      true,
    ContentTypeImage:     true,
    ContentTypeVideo:     true,
    ContentTypeFile:      true,
    ContentTypeAudio:     true,
    ContentTypeTextImage: true,
    ContentTypeTextVideo: true,
    ContentTypeTextFile:  true,
    ContentTypeMultiple:  true,
    ContentTypeDeleted:   true,
}


func IsValidContentType(contentType string) bool {
    _, exists := ValidContentTypes[contentType]
    return exists
}