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
	IsMarketplace  bool   `json:"is_marketplace"` // For distinguishing marketplace vs personal DM typing
	Timestamp      int64  `json:"timestamp"`
}

// DeliveredEvent represents a message delivery event
type DeliveredEvent struct {
	MessageIDs  []primitive.ObjectID `json:"message_ids"`
	DelivererID primitive.ObjectID   `json:"deliverer_id"` // The user who received the message
	Timestamp   time.Time            `json:"timestamp"`
}

// // ReadReceiptEvent represents an event where one or more messages have been seen by a user.
// type ReadReceiptEvent struct {
// 	MessageIDs []primitive.ObjectID `json:"message_ids"`
// 	ReaderID   primitive.ObjectID   `json:"reader_id"`
// 	Timestamp  time.Time            `json:"timestamp"`
// }

// ConversationSeenEvent represents an event where a whole conversation has been seen by a user up to a certain timestamp.
type ConversationSeenEvent struct {
	ConversationID   primitive.ObjectID `json:"conversation_id"`
	ConversationUIID string             `json:"conversation_ui_id"`
	UserID           primitive.ObjectID `json:"user_id"`
	Timestamp        time.Time          `json:"timestamp"`
	IsGroup          bool               `json:"is_group"`
}

// CallSignalEvent represents a signaling message for voice/video calls.
type CallSignalEvent struct {
	TargetID   string          `json:"target_id"`
	SignalType string          `json:"signal_type"` // OFFER, ANSWER, ICE_CANDIDATE, END_CALL, REJECT_CALL, BUSY
	SignalData json.RawMessage `json:"signal_data,omitempty"`
	CallerID   string          `json:"caller_id,omitempty"` // Added by server when forwarding
	CallType   string          `json:"call_type,omitempty"` // 'audio' or 'video'
}
