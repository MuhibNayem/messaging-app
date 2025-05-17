package websocket

import (
	"context"
	"encoding/json"
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/redis"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	wsConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_connections_total",
		Help: "Current number of active WebSocket connections",
	})
	wsMessagesSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "websocket_messages_sent_total",
		Help: "Total number of messages sent via WebSocket",
	}, []string{"type"})
)

func init() {
	prometheus.MustRegister(wsConnections, wsMessagesSent)
}

type Client struct {
	UserID    string
	Conn      *websocket.Conn
	Send      chan []byte
	LastSeen  time.Time
	Listeners map[string]bool // Channels/groups this client is listening to
	ActiveConversations map[string]bool
}

type Hub struct {
	Clients      map[*Client]bool
	RedisClient  *redis.ClusterClient
	Broadcast    chan models.Message
	Register     chan *Client
	Unregister   chan *Client
	mu           sync.RWMutex
	messageCache *MessageCache
	TypingEvents chan models.TypingEvent
}

type MessageCache struct {
	redis *redis.ClusterClient
}

func NewMessageCache(redis *redis.ClusterClient) *MessageCache {
	return &MessageCache{redis: redis}
}

func (mc *MessageCache) Store(ctx context.Context, msg models.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return mc.redis.Set(ctx, "msg:"+msg.ID.Hex(), data, 24*time.Hour)
}

func (mc *MessageCache) Get(ctx context.Context, msgID string) (*models.Message, error) {
	data, err := mc.redis.Get(ctx, "msg:"+msgID)
	if err != nil {
		return nil, err
	}
	var msg models.Message
	err = json.Unmarshal([]byte(data), &msg)
	return &msg, err
}

func NewHub(redisClient *redis.ClusterClient) *Hub {
	hub := &Hub{
		Broadcast:    make(chan models.Message, 10000),
		Register:     make(chan *Client, 1000),
		Unregister:   make(chan *Client, 1000),
		Clients:      make(map[*Client]bool),
		RedisClient:  redisClient,
		messageCache: NewMessageCache(redisClient),
		TypingEvents: make(chan models.TypingEvent, 1000),
	}

	// Subscribe to Redis for cross-instance messages
	go hub.subscribeToRedis()
	go hub.cleanupStaleConnections()

	return hub
}

func (h *Hub) cleanupStaleConnections() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		for client := range h.Clients {
			if time.Since(client.LastSeen) > 10*time.Minute {
				close(client.Send)
				delete(h.Clients, client)
				wsConnections.Dec()
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) subscribeToRedis() {
	pubsub := h.RedisClient.Subscribe(context.Background(), "messages")
	defer pubsub.Close()

	channel := pubsub.Channel()
	for msg := range channel {
		var message models.Message
		if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
			log.Printf("Error unmarshaling Redis message: %v", err)
			continue
		}
		h.Broadcast <- message
	}
}

func (h *Hub) Run() {
	for {
		select {
        
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client] = true
			wsConnections.Inc()
			h.mu.Unlock()

			// Send any cached messages for this user
			go h.sendCachedMessages(client)

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client]; ok {
				close(client.Send)
				delete(h.Clients, client)
				wsConnections.Dec()
			}
			h.mu.Unlock()

		case message := <-h.Broadcast:
			// Cache the message first
			if err := h.messageCache.Store(context.Background(), message); err != nil {
				log.Printf("Failed to cache message: %v", err)
			}

			h.mu.RLock()
			for client := range h.Clients {
				// Check if client is interested in this message
				if client.shouldReceive(message) {
					jsonMsg, err := json.Marshal(message)
					if err != nil {
						log.Printf("Error marshaling message: %v", err)
						continue
					}

					select {
					case client.Send <- jsonMsg:
						client.LastSeen = time.Now()
						wsMessagesSent.WithLabelValues(message.ContentType).Inc()
					default:
						close(client.Send)
						delete(h.Clients, client)
						wsConnections.Dec()
					}
				}
			}
			h.mu.RUnlock()
		case typingEvent := <-h.TypingEvents:
            h.broadcastTypingEvent(typingEvent)
		}
	}
}

func (h *Hub) broadcastTypingEvent(event models.TypingEvent) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    for client := range h.Clients {
        // Send to:
        // 1. The other participant in direct messages
        // 2. All group members except sender
        if client.isInterestedInTypingEvent(event) {
            jsonMsg, err := json.Marshal(event)
            if err != nil {
                log.Printf("Error marshaling typing event: %v", err)
                continue
            }
            
            select {
            case client.Send <- jsonMsg:
                client.LastSeen = time.Now()
            default:
                close(client.Send)
                delete(h.Clients, client)
                wsConnections.Dec()
            }
        }
    }
}

func (c *Client) isInterestedInTypingEvent(event models.TypingEvent) bool {
    // Don't send to self
    if event.UserID == c.UserID {
        return false
    }
    
    // Check if client is in this conversation
    if _, ok := c.ActiveConversations[event.ConversationID]; ok {
        return true
    }
    
    return false
}

func (c *Client) shouldReceive(msg models.Message) bool {
	// Check direct message
	if !msg.ReceiverID.IsZero() && (msg.ReceiverID.Hex() == c.UserID || msg.SenderID.Hex() == c.UserID) {
		return true
	}

	// Check group message
	if !msg.GroupID.IsZero() {
		if _, ok := c.Listeners[msg.GroupID.Hex()]; ok {
			return true
		}
	}

	return false
}

func (h *Hub) sendCachedMessages(client *Client) {
	// In a real implementation, you'd fetch unread messages from cache/database
	// and send them to the newly connected client
}

func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Get user ID from JWT or other auth mechanism
	userID := r.URL.Query().Get("userID")
	if userID == "" {
		conn.Close()
		return
	}

	// Get channels/groups this user is subscribed to
	// This would typically come from a database or cache
	listeners := make(map[string]bool)
	// Example: add direct messaging channel
	listeners[userID] = true

	client := &Client{
		UserID:    userID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		LastSeen:  time.Now(),
		Listeners: listeners,
	}

	hub.Register <- client

	// Read pump
	go func() {
		defer func() {
			hub.Unregister <- client
			conn.Close()
		}()

		for {
			_, message, err := conn.ReadMessage()
			log.Printf("Received message of length: %d", len(message))
			if err != nil {
				break
			}
			// Handle incoming WebSocket messages (e.g., typing indicators)
			// This could be extended to support more real-time features
		}
	}()

	// Write pump
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer func() {
			ticker.Stop()
			conn.Close()
		}()

		for {
			select {
			case message, ok := <-client.Send:
				if !ok {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					return
				}
			case <-ticker.C:
				// Send ping to keep connection alive
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}