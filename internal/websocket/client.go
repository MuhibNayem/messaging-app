package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"messaging-app/internal/models"

	"github.com/gorilla/websocket"
)

// Client represents a single websocket connection.
type Client struct {
	userID    string
	conn      *websocket.Conn
	send      chan []byte
	lastSeen  time.Time
	mu        sync.RWMutex // protects lastSeen
	listeners map[string]bool
	Status    string
}

// readPump pumps messages from the websocket connection to the Hub.
func (c *Client) readPump(h *Hub) {
	const (
		pongWait   = 60 * time.Second
		maxMsgSize = 32768 // Increased to 32KB to handle WebRTC SDP
	)
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMsgSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		c.setLastSeen(time.Now())
		return nil
	})

	for {
		_, msgBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS read error: %v", err)
			}
			break
		}

		var env struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(msgBytes, &env); err != nil {
			log.Printf("Invalid message: %v", err)
			continue
		}

		switch env.Type {
		case "typing":
			var typingData struct {
				ConversationID string `json:"conversation_id"`
				IsTyping       bool   `json:"isTyping"`
				IsMarketplace  bool   `json:"is_marketplace"`
			}
			if err := json.Unmarshal(env.Payload, &typingData); err != nil {
				log.Printf("Error unmarshaling typing data: %v", err)
				return
			}
			h.typingEvents <- models.TypingEvent{
				UserID:         c.userID,
				ConversationID: typingData.ConversationID,
				IsTyping:       typingData.IsTyping,
				IsMarketplace:  typingData.IsMarketplace,
				Timestamp:      time.Now().Unix(),
			}
		case "message":
			var m models.Message
			if err := json.Unmarshal(env.Payload, &m); err == nil && m.Content != "" && m.SenderID.Hex() == c.userID {
				h.Broadcast <- m
			}
		case "call_signal":
			var signal models.CallSignalEvent
			if err := json.Unmarshal(env.Payload, &signal); err != nil {
				log.Printf("Error unmarshaling call signal: %v", err)
				return
			}
			signal.CallerID = c.userID // Ensure CallerID is set to the vetted user
			h.CallSignal <- signal
		case "presence":
			c.setLastSeen(time.Now())
		default:
			log.Printf("Unknown type: %s", env.Type)
		}
	}
}

// writePump pumps messages from the Hub to the websocket connection.
func (c *Client) writePump() {
	const pingPeriod = (60 * time.Second * 9) / 10
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) setLastSeen(t time.Time) {
	c.mu.Lock()
	c.lastSeen = t
	c.mu.Unlock()
}
