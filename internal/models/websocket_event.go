package models

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// WebSocketEvent is a generic structure for events sent over WebSocket.
// It contains a Type field to identify the event and Data for the event-specific payload.
type WebSocketEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// TypingEvent represents a user typing event in a conversation.
type TypingEvent struct {
	UserID         string `json:"user_id"`
	ConversationID string `json:"conversation_id"`
	IsTyping       bool   `json:"is_typing"`
	Timestamp      int64  `json:"timestamp"`
}

// DeliveredEvent represents a message delivery event
type DeliveredEvent struct {
	MessageIDs  []primitive.ObjectID `json:"message_ids"`
	DelivererID primitive.ObjectID   `json:"deliverer_id"` // The user who received the message
	Timestamp   time.Time            `json:"timestamp"`
}
