package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NotificationType defines the type of notification (e.g., mention, like, comment)
type NotificationType string

const (
	NotificationTypeMention             NotificationType = "MENTION"
	NotificationTypeLike                NotificationType = "LIKE"
	NotificationTypeComment             NotificationType = "COMMENT"
	NotificationTypeReply               NotificationType = "REPLY"
	NotificationTypeFriendRequest       NotificationType = "FRIEND_REQUEST"
	NotificationTypeFriendAccept        NotificationType = "FRIEND_ACCEPT"
	NotificationTypeBirthday            NotificationType = "BIRTHDAY"
	NotificationTypeEventInvite         NotificationType = "EVENT_INVITE"
	NotificationTypeEventReminder       NotificationType = "EVENT_REMINDER"
	NotificationTypeEventInviteAccepted NotificationType = "EVENT_INVITE_ACCEPTED"
	NotificationTypeEventInviteDeclined NotificationType = "EVENT_INVITE_DECLINED"
)

// Notification represents a single notification for a user
type Notification struct {
	ID          primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	RecipientID primitive.ObjectID     `bson:"recipient_id" json:"recipient_id"` // The user who receives the notification
	SenderID    primitive.ObjectID     `bson:"sender_id" json:"sender_id"`       // The user who triggered the notification
	Type        NotificationType       `bson:"type" json:"type"`
	TargetID    primitive.ObjectID     `bson:"target_id" json:"target_id"`           // ID of the related entity (post, comment, reply, etc.)
	TargetType  string                 `bson:"target_type" json:"target_type"`       // Type of the related entity ("post", "comment", "reply", "friendship")
	Content     string                 `bson:"content" json:"content"`               // A short message for the notification
	Data        map[string]interface{} `bson:"data,omitempty" json:"data,omitempty"` // Structured data for the notification
	Read        bool                   `bson:"read" json:"read"`
	CreatedAt   time.Time              `bson:"created_at" json:"created_at"`
}

// DTOs for Notifications
type CreateNotificationRequest struct {
	RecipientID primitive.ObjectID     `json:"recipient_id" binding:"required"`
	SenderID    primitive.ObjectID     `json:"sender_id" binding:"required"`
	Type        NotificationType       `json:"type" binding:"required"`
	TargetID    primitive.ObjectID     `json:"target_id" binding:"required"`
	TargetType  string                 `json:"target_type" binding:"required"`
	Content     string                 `json:"content" binding:"required"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

type UpdateNotificationRequest struct {
	Read bool `json:"read"`
}

type NotificationListResponse struct {
	Notifications []Notification `json:"notifications"`
	Total         int64          `json:"total"`
	Page          int64          `json:"page"`
	Limit         int64          `json:"limit"`
}
