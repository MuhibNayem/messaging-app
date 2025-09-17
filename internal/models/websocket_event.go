package models

import "encoding/json"

// WebSocketEvent is a generic structure for events sent over WebSocket.
// It contains a Type field to identify the event and Data for the event-specific payload.
type WebSocketEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
