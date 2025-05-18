package websocket

import (
	"context"
	"encoding/json"
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/redis"
	"messaging-app/internal/repositories"
	"messaging-app/pkg/utils"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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
	UserID               string
	Conn                 *websocket.Conn
	Send                 chan []byte
	LastSeen             time.Time
	Listeners            map[string]bool 
	ActiveConversations  map[string]bool
}

type Hub struct {
	Clients       	map[*Client]bool
	GroupRepository *repositories.GroupRepository
	RedisClient   	*redis.ClusterClient
	Broadcast     	chan models.Message
	Register      	chan *Client
	Unregister    	chan *Client
	mu            	sync.RWMutex
	messageCache  	*MessageCache
	TypingEvents  	chan models.TypingEvent
}

type MessageCache struct {
	redis *redis.ClusterClient
}

func NewMessageCache(redis *redis.ClusterClient) *MessageCache {
	return &MessageCache{redis: redis}
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

func NewHub(redisClient *redis.ClusterClient, GroupRepository *repositories.GroupRepository) *Hub {
	hub := &Hub{
		Broadcast:    make(chan models.Message, 10000),
		Register:     make(chan *Client, 1000),
		Unregister:   make(chan *Client, 1000),
		Clients:      make(map[*Client]bool),
		RedisClient:  redisClient,
		GroupRepository: GroupRepository,
		messageCache: NewMessageCache(redisClient),
		TypingEvents: make(chan models.TypingEvent, 1000),
	}

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
			if err := h.messageCache.Store(context.Background(), message); err != nil {
				log.Printf("Failed to cache message: %v", err)
			}

			if !message.GroupID.IsZero() {
				h.handleGroupMessage(message)
			}

			h.mu.RLock()
			for client := range h.Clients {
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

func (h *Hub) handleGroupMessage(msg models.Message) {
	members, err := h.getGroupMembers(msg.GroupID.Hex())
	if err != nil {
		log.Printf("Error getting group members: %v", err)
		return
	}

	onlineMembers := make(map[string]bool)
	h.mu.RLock()
	for client := range h.Clients {
		onlineMembers[client.UserID] = true
	}
	h.mu.RUnlock()

	for _, memberID := range members {
		if !onlineMembers[memberID] {
			err := h.messageCache.AddPendingGroupMessage(
				context.Background(),
				memberID,
				msg.ID.Hex(),
			)
			if err != nil {
				log.Printf("Failed to add group pending message: %v", err)
			}
		}
	}
}

func (h *Hub) getGroupMembers(groupID string) ([]string, error) {
	return h.RedisClient.SMembers(context.Background(), "group:members:"+groupID).Result()
}

func (h *Hub) broadcastTypingEvent(event models.TypingEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	for client := range h.Clients {
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
	if event.UserID == c.UserID {
		return false
	}
	
	if _, ok := c.ActiveConversations[event.ConversationID]; ok {
		return true
	}
	
	return false
}

func (c *Client) shouldReceive(msg models.Message) bool {
	if !msg.ReceiverID.IsZero() && (msg.ReceiverID.Hex() == c.UserID || msg.SenderID.Hex() == c.UserID) {
		return true
	}

	if !msg.GroupID.IsZero() {
		if _, ok := c.Listeners[msg.GroupID.Hex()]; ok {
			return true
		}
	}

	return false
}

func (mc *MessageCache) Store(ctx context.Context, msg models.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	err = mc.redis.Set(ctx, "msg:"+msg.ID.Hex(), data, 24*time.Hour)
	if err != nil {
		return err
	}

	if !msg.ReceiverID.IsZero() {
		return mc.AddPendingDirectMessage(ctx, msg.ReceiverID.Hex(), msg.ID.Hex())
	} else if !msg.GroupID.IsZero() {
		return mc.AddPendingGroupMessage(ctx, msg.GroupID.Hex(), msg.ID.Hex())
	}
	
	return nil
}

func (mc *MessageCache) AddPendingDirectMessage(ctx context.Context, userID, messageID string) error {
	key := "pending:direct:" + userID
	return mc.redis.SAdd(ctx, key, messageID).Err()
}

func (mc *MessageCache) GetPendingDirectMessages(ctx context.Context, userID string) ([]string, error) {
	key := "pending:direct:" + userID
	return mc.redis.SMembers(ctx, key).Result()
}

func (mc *MessageCache) RemovePendingDirectMessage(ctx context.Context, userID, messageID string) error {
	key := "pending:direct:" + userID
	return mc.redis.SRem(ctx, key, messageID).Err()
}

func (mc *MessageCache) AddPendingGroupMessage(ctx context.Context, groupID, messageID string) error {
	key := "pending:group:" + groupID
	return mc.redis.SAdd(ctx, key, messageID).Err()
}

func (mc *MessageCache) GetPendingGroupMessages(ctx context.Context, groupID string) ([]string, error) {
	key := "pending:group:" + groupID
	return mc.redis.SMembers(ctx, key).Result()
}

func (mc *MessageCache) RemovePendingGroupMessage(ctx context.Context, groupID, messageID string) error {
	key := "pending:group:" + groupID
	return mc.redis.SRem(ctx, key, messageID).Err()
}

func (h *Hub) sendCachedMessages(client *Client) {
	ctx := context.Background()

	// Handle direct messages
	directMsgIDs, err := h.messageCache.GetPendingDirectMessages(ctx, client.UserID)
	if err != nil {
		log.Printf("Error fetching direct messages: %v", err)
	} else {
		h.sendPendingMessages(client, directMsgIDs, "direct")
	}

	// Handle group messages for all subscribed groups
	for groupID := range client.Listeners {
		if groupID == client.UserID {
			continue
		}
		
		groupMsgIDs, err := h.messageCache.GetPendingGroupMessages(ctx, groupID)
		if err != nil {
			log.Printf("Error fetching group messages: %v", err)
			continue
		}
		
		h.sendPendingMessages(client, groupMsgIDs, "group")
	}
}

func (h *Hub) sendPendingMessages(client *Client, msgIDs []string, msgType string) {
	for _, msgID := range msgIDs {
		msg, err := h.messageCache.Get(context.Background(), msgID)
		if err != nil {
			log.Printf("Error retrieving message %s: %v", msgID, err)
			continue
		}

		if msgType == "direct" && msg.ReceiverID.Hex() != client.UserID {
			continue
		}
		if msgType == "group" && !client.Listeners[msg.GroupID.Hex()] {
			continue
		}

		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Error marshaling message %s: %v", msgID, err)
			continue
		}

		select {
		case client.Send <- jsonMsg:
			if msgType == "direct" {
				h.messageCache.RemovePendingDirectMessage(context.Background(), client.UserID, msgID)
			} else {
				h.messageCache.RemovePendingGroupMessage(context.Background(), msg.GroupID.Hex(), msgID)
			}
			wsMessagesSent.WithLabelValues(msg.ContentType).Inc()
		default:
			log.Printf("Client channel full, skipping cached message")
		}
	}
}

func ServeWs(ctx *gin.Context, hub *Hub, w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	userID := r.URL.Query().Get("userID")
	userIDFromContext, err := utils.GetUserIDFromContext(ctx)
	if userID == "" {
		conn.Close()
		return
	}

	listeners := make(map[string]bool)
	listeners[userID] = true

	// Get user's groups from database/redis
	groups, err := hub.GroupRepository.GetUserGroups(ctx, userIDFromContext)
	for _, group := range groups {
		groupID := group.ID.Hex()
		listeners[groupID] = true
	}

	client := &Client{
		UserID:    userID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		LastSeen:  time.Now(),
		Listeners: listeners,
	}

	hub.Register <- client

	go func() {
		defer func() {
			hub.Unregister <- client
			conn.Close()
		}()
	
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				break
			}
	
			var msg struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
	
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Error parsing message: %v", err)
				continue
			}
	
			switch msg.Type {
			case "typing":
				var typingEvent struct {
					ConversationID string `json:"conversationId"`
					IsTyping       bool   `json:"isTyping"`
				}
				
				if err := json.Unmarshal(msg.Payload, &typingEvent); err != nil {
					log.Printf("Error parsing typing event: %v", err)
					continue
				}
	
				// Validate conversation ID format
				if typingEvent.ConversationID == "" {
					log.Printf("Invalid conversation ID in typing event")
					continue
				}
	
				// Send to hub for processing
				hub.TypingEvents <- models.TypingEvent{
					UserID:         client.UserID,
					ConversationID: typingEvent.ConversationID,
					IsTyping:       typingEvent.IsTyping,
					Timestamp:      time.Now().Unix(),
				}
	
			case "message":
				// Example for handling direct messages through WS
				var chatMsg models.Message
				if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
					log.Printf("Error parsing chat message: %v", err)
					continue
				}
				
				// Validate message
				if chatMsg.Content == "" || chatMsg.SenderID.Hex() != client.UserID {
					log.Printf("Invalid message received")
					continue
				}
				
				// Process message through normal pipeline
				hub.Broadcast <- chatMsg
	
			case "presence":
				// Example presence update handling
				var presence struct {
					Status    string `json:"status"`
					Timestamp int64  `json:"timestamp"`
				}
				if err := json.Unmarshal(msg.Payload, &presence); err != nil {
					log.Printf("Error parsing presence update: %v", err)
					continue
				}
				
				// Update last seen and handle presence
				client.LastSeen = time.Now()
				// Add additional presence handling logic here
	
			default:
				log.Printf("Unknown message type: %s", msg.Type)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

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
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}
