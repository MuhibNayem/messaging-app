package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MessageReaction represents a single MessageReaction to a message
type MessageReaction struct {
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Emoji     string             `bson:"emoji" json:"emoji"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
}

// MessageProduct is a lightweight product representation for message embedding
type MessageProduct struct {
	ID       primitive.ObjectID `bson:"_id" json:"id"`
	Title    string             `bson:"title" json:"title"`
	Price    float64            `bson:"price" json:"price"`
	Currency string             `bson:"currency" json:"currency"`
	Images   []string           `bson:"images" json:"images"`
	Status   string             `bson:"status" json:"status"`
}

type Message struct {
	ID               primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	StringID         string               `bson:"string_id,omitempty" json:"string_id,omitempty"` // For Cassandra UUID mapping
	SenderID         primitive.ObjectID   `bson:"sender_id" json:"sender_id"`
	SenderName       string               `bson:"sender_name,omitempty" json:"sender_name,omitempty"`
	ReceiverID       primitive.ObjectID   `bson:"receiver_id,omitempty" json:"receiver_id,omitempty"`
	GroupID          primitive.ObjectID   `bson:"group_id,omitempty" json:"group_id,omitempty"`
	GroupName        string               `bson:"group_name,omitempty" json:"group_name,omitempty"`
	Content          string               `bson:"content,omitempty" json:"content,omitempty"`
	ContentType      string               `bson:"content_type" json:"content_type"`
	MediaURLs        []string             `bson:"media_urls,omitempty" json:"media_urls,omitempty"`
	SeenBy           []primitive.ObjectID `bson:"seen_by" json:"seen_by"`
	DeliveredTo      []primitive.ObjectID `bson:"delivered_to" json:"delivered_to"`
	IsDeleted        bool                 `bson:"is_deleted" json:"is_deleted"`
	DeletedAt        *time.Time           `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	OriginalContent  string               `bson:"original_content,omitempty" json:"-"`
	IsEdited         bool                 `bson:"is_edited" json:"is_edited"`                                         // New field for message editing
	EditedAt         *time.Time           `bson:"edited_at,omitempty" json:"edited_at,omitempty"`                     // New field for message editing
	Reactions        []MessageReaction    `bson:"reactions,omitempty" json:"reactions,omitempty"`                     // New field for reactions
	ReplyToMessageID *primitive.ObjectID  `bson:"reply_to_message_id,omitempty" json:"reply_to_message_id,omitempty"` // New field for replies
	ProductID        *primitive.ObjectID  `bson:"product_id,omitempty" json:"product_id,omitempty"`                   // New field for marketplace inquiries
	IsMarketplace    bool                 `bson:"is_marketplace" json:"is_marketplace"`                               // Flag for marketplace context
	Product          *MessageProduct      `bson:"product,omitempty" json:"product,omitempty"`                         // Populated product data
	Mentions         []primitive.ObjectID `bson:"mentions,omitempty" json:"mentions,omitempty"`
	MentionedUsers   []PostAuthor         `bson:"-" json:"mentioned_users,omitempty"`
	Sender           *SafeUserResponse    `bson:"sender,omitempty" json:"sender,omitempty"`
	CreatedAt        time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time            `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	IsEncrypted      bool                 `bson:"is_encrypted" json:"is_encrypted"`                         // E2EE
	IV               string               `bson:"iv,omitempty" json:"iv,omitempty"`                         // E2EE
	EncryptedKeys    map[string]string    `bson:"encrypted_keys,omitempty" json:"encrypted_keys,omitempty"` // E2EE
}

type MessageQuery struct {
	GroupID        string `form:"group_id"`
	SenderID       string `form:"sender_id"`
	ReceiverID     string `form:"receiver_id"`
	ConversationID string `form:"conversation_id"` // New field
	Page           int    `form:"page,default=1"`
	Limit          int    `form:"limit,default=50"`
	Before         string `form:"before"`
	Marketplace    bool   `form:"marketplace"` // If true, only return messages with product_id
}

type MessageRequest struct {
	SenderName       string   `bson:"sender_name,omitempty" json:"sender_name,omitempty" form:"sender_name"`
	ReceiverID       string   `json:"receiver_id,omitempty" form:"receiver_id"`
	SenderID         string   `json:"sender_id" form:"sender_id"`
	GroupID          string   `json:"group_id,omitempty" form:"group_id"`
	Content          string   `json:"content,omitempty" form:"content"`
	ContentType      string   `json:"content_type" form:"content_type"`
	MediaURLs        []string `json:"media_urls,omitempty" form:"media_urls"`
	ReplyToMessageID string   `json:"reply_to_message_id,omitempty" form:"reply_to_message_id"` // New field for replies
	ProductID        string   `json:"product_id,omitempty" form:"product_id"`                   // New field for marketplace inquiries
	IsMarketplace    bool     `json:"is_marketplace" form:"is_marketplace"`                     // Flag for marketplace context
	IsEncrypted      bool     `json:"is_encrypted" form:"is_encrypted"`
	IV               string   `json:"iv,omitempty" form:"iv"`
	EncryptedKeys    string   `json:"encrypted_keys,omitempty" form:"encrypted_keys"` // JSON string for map
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

// ReactionEvent represents a Kafka event for message reactions
type ReactionEvent struct {
	MessageID primitive.ObjectID `json:"message_id"`
	UserID    primitive.ObjectID `json:"user_id"`
	Emoji     string             `json:"emoji"`
	Action    string             `json:"action"` // "add" or "remove"
	Timestamp time.Time          `json:"timestamp"`
}

// ReadReceiptEvent represents a Kafka event for message read receipts
type ReadReceiptEvent struct {
	MessageIDs []primitive.ObjectID `json:"message_ids"`
	ReaderID   primitive.ObjectID   `json:"reader_id"`
	Timestamp  time.Time            `json:"timestamp"`
}

// MessageEditedEvent represents a Kafka event for message edits
type MessageEditedEvent struct {
	MessageID  primitive.ObjectID `json:"message_id"`
	EditorID   primitive.ObjectID `json:"editor_id"`
	NewContent string             `json:"new_content"`
	EditedAt   time.Time          `json:"edited_at"`
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
	ContentTypeProduct   = "product" // New content type for marketplace inquiries
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
	ContentTypeProduct:   true,
}

func IsValidContentType(contentType string) bool {
	_, exists := ValidContentTypes[contentType]
	return exists
}

// ConversationSummary represents a summary of a chat conversation for the list view
type ConversationSummary struct {
	ID                     string             `bson:"_id" json:"id"`
	Name                   string             `bson:"name" json:"name"`
	Avatar                 string             `bson:"avatar" json:"avatar,omitempty"`
	IsGroup                bool               `bson:"is_group" json:"is_group"`
	LastMessageSenderID    primitive.ObjectID `bson:"last_message_sender_id" json:"last_message_sender_id,omitempty"`
	LastMessageSenderName  string             `bson:"last_message_sender_name" json:"last_message_sender_name,omitempty"`
	LastMessageContent     string             `bson:"last_message_content" json:"last_message_content,omitempty"`
	LastMessageTimestamp   *time.Time         `bson:"last_message_timestamp" json:"last_message_timestamp,omitempty"`
	LastMessageIsEncrypted bool               `bson:"last_message_is_encrypted" json:"last_message_is_encrypted"`
	UnreadCount            int64              `bson:"unread_count" json:"unread_count"`
}
