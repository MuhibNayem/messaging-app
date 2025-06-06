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
	pendingDirectMessages = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pending_direct_messages_total",
		Help: "Number of pending direct messages",
	})
	pendingGroupMessages = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pending_group_messages_total",
		Help: "Number of pending group messages",
	})
	broadcastLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "websocket_broadcast_latency_seconds",
		Help:    "Time from message received to send",
		Buckets: prometheus.DefBuckets,
	})
)

func init() {
	prometheus.MustRegister(
		wsConnections,
		wsMessagesSent,
		pendingDirectMessages,
		pendingGroupMessages,
		broadcastLatency,
	)
}

// Client represents a single websocket connection
type Client struct {
	userID    string
	conn      *websocket.Conn
	send      chan []byte
	lastSeen  time.Time
	mu        sync.RWMutex // protects lastSeen
	listeners map[string]bool
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	userClients  map[string]map[*Client]bool
	groupClients map[string]map[*Client]bool

	groupRepo    *repositories.GroupRepository
	redisClient  *redis.ClusterClient
	messageCache *MessageCache

	register     chan *Client
	unregister   chan *Client
	Broadcast    chan models.Message
	typingEvents chan models.TypingEvent

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex
}

// NewHub creates a new Hub and starts its goroutines
func NewHub(redisClient *redis.ClusterClient, groupRepo *repositories.GroupRepository) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Hub{
		userClients:  make(map[string]map[*Client]bool),
		groupClients: make(map[string]map[*Client]bool),
		groupRepo:    groupRepo,
		redisClient:  redisClient,
		messageCache: NewMessageCache(redisClient),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		Broadcast:    make(chan models.Message, 10000),
		typingEvents: make(chan models.TypingEvent, 1000),
		ctx:          ctx,
		cancel:       cancel,
	}
	go h.run()
	go h.subscribeToRedis()
	go h.cleanupStaleConnections()
	return h
}

func (h *Hub) run() {
	for {
		select {
		case <-h.ctx.Done():
			return

		case c := <-h.register:
			h.addClient(c)
			go h.sendCachedMessages(c)

		case c := <-h.unregister:
			h.removeClient(c)

		case msg := <-h.Broadcast:
			start := time.Now()
			if err := h.messageCache.Store(h.ctx, msg); err != nil {
				log.Printf("Failed to cache message: %v", err)
			}
			h.dispatchMessage(msg)
			broadcastLatency.Observe(time.Since(start).Seconds())

		case ev := <-h.typingEvents:
			h.dispatchTypingEvent(ev)
		}
	}
}

func (h *Hub) addClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.userClients[c.userID]; !ok {
		h.userClients[c.userID] = make(map[*Client]bool)
	}
	h.userClients[c.userID][c] = true
	for gid := range c.listeners {
		if _, ok := h.groupClients[gid]; !ok {
			h.groupClients[gid] = make(map[*Client]bool)
		}
		h.groupClients[gid][c] = true
	}
	wsConnections.Inc()
}

func (h *Hub) removeClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// remove from user map
	if conns, ok := h.userClients[c.userID]; ok {
		if _, exists := conns[c]; exists {
			delete(conns, c)
			if len(conns) == 0 {
				delete(h.userClients, c.userID)
			}
		}
	}
	// remove from group maps
	for gid := range c.listeners {
		if conns, ok := h.groupClients[gid]; ok {
			if _, exists := conns[c]; exists {
				delete(conns, c)
				if len(conns) == 0 {
					delete(h.groupClients, gid)
				}
			}
		}
	}
	wsConnections.Dec()
	close(c.send)
}

func (h *Hub) dispatchMessage(msg models.Message) {
	// direct
	if !msg.ReceiverID.IsZero() {
		h.sendToClients(h.getClientsByUser(msg.ReceiverID.Hex()), msg)
		return
	}
	// group
	if !msg.GroupID.IsZero() {
		h.sendToClients(h.getClientsByGroup(msg.GroupID.Hex()), msg)
		h.queuePendingForGroup(msg)
	}
}

func (h *Hub) sendToClients(clients []*Client, msg models.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	for _, c := range clients {
		select {
		case c.send <- data:
			c.setLastSeen(time.Now())
			wsMessagesSent.WithLabelValues(msg.ContentType).Inc()
		default:
			h.removeClient(c)
		}
	}
}

func (h *Hub) queuePendingForGroup(msg models.Message) {
	members, err := h.getGroupMembers(msg.GroupID.Hex())
	if err != nil {
		log.Printf("Error getting group members: %v", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, uid := range members {
		if _, online := h.userClients[uid]; !online {
			if err := h.messageCache.AddPendingDirectMessage(h.ctx, uid, msg.ID.Hex()); err != nil {
				log.Printf("Failed to queue pending for %s: %v", uid, err)
			}
		}
	}
}

func (h *Hub) getClientsByUser(uid string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var list []*Client
	for c := range h.userClients[uid] {
		list = append(list, c)
	}
	return list
}

func (h *Hub) getClientsByGroup(gid string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var list []*Client
	for c := range h.groupClients[gid] {
		list = append(list, c)
	}
	return list
}

// sendCachedMessages pushes any pending direct and group messages
// to the newly registered client.
func (h *Hub) sendCachedMessages(client *Client) {
    ctx := h.ctx

    directIDs, err := h.messageCache.GetPendingDirectMessages(ctx, client.userID)
    if err != nil {
        log.Printf("Error fetching direct messages: %v", err)
    } else {
        h.sendPendingMessages(client, directIDs, "direct")
    }

    for groupID := range client.listeners {
        if groupID == client.userID {
            continue
        }
        groupIDs, err := h.messageCache.GetPendingGroupMessages(ctx, groupID)
        if err != nil {
            log.Printf("Error fetching group messages: %v", err)
            continue
        }
        h.sendPendingMessages(client, groupIDs, "group")
    }
}

// sendPendingMessages delivers stored messages and cleans up the pending sets.
func (h *Hub) sendPendingMessages(client *Client, msgIDs []string, msgType string) {
    ctx := h.ctx

    for _, id := range msgIDs {
        msg, err := h.messageCache.Get(ctx, id)
        if err != nil {
            log.Printf("Error retrieving message %s: %v", id, err)
            continue
        }

        // 2) basic delivery check
        if msgType == "direct" {
            if msg.ReceiverID.Hex() != client.userID {
                continue
            }
        } else { 
            if !client.listeners[msg.GroupID.Hex()] {
                continue
            }
        }

        // 3) marshal & send
        data, err := json.Marshal(msg)
        if err != nil {
            log.Printf("Error marshaling message %s: %v", id, err)
            continue
        }

        select {
        case client.send <- data:
            if msgType == "direct" {
                if err := h.messageCache.RemovePendingDirectMessage(ctx, client.userID, id); err == nil {
                    pendingDirectMessages.Dec()
                }
            } else {
                if err := h.messageCache.RemovePendingGroupMessage(ctx, msg.GroupID.Hex(), id); err == nil {
                    pendingGroupMessages.Dec()
                }
            }
            wsMessagesSent.WithLabelValues(msg.ContentType).Inc()

        default:
            log.Printf("Client channel full, skipping cached message")
        }
    }
}


func (h *Hub) dispatchTypingEvent(ev models.TypingEvent) {
	clients := h.getClientsByGroup(ev.ConversationID)
	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("Error marshaling typing event: %v", err)
		return
	}
	for _, c := range clients {
		if c.userID == ev.UserID {
			continue
		}
		select {
		case c.send <- data:
			c.setLastSeen(time.Now())
		default:
			h.removeClient(c)
		}
	}
}

func (h *Hub) cleanupStaleConnections() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute)
			var stale []*Client

			h.mu.RLock()
			for _, conns := range h.userClients {
				for c := range conns {
					c.mu.RLock()
					last := c.lastSeen
					c.mu.RUnlock()
					if last.Before(cutoff) {
						stale = append(stale, c)
					}
				}
			}
			h.mu.RUnlock()
			for _, c := range stale {
				h.removeClient(c)
			}
		}
	}
}

func (h *Hub) subscribeToRedis() {
	pubsub := h.redisClient.Subscribe(h.ctx, "messages")
	defer pubsub.Close()
	ch := pubsub.Channel()
	for {
		select {
		case <-h.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var m models.Message
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				log.Printf("Error unmarshaling Redis message: %v", err)
				continue
			}
			h.Broadcast <- m
		}
	}
}

func (h *Hub) getGroupMembers(groupID string) ([]string, error) {
	return h.redisClient.SMembers(context.Background(), "group:members:"+groupID).Result()
}

// MessageCache handles storing and retrieving messages and pending queues


type MessageCache struct {
	redis *redis.ClusterClient
}

func NewMessageCache(redisClient *redis.ClusterClient) *MessageCache {
	return &MessageCache{redis: redisClient}
}

func (mc *MessageCache) Store(ctx context.Context, msg models.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	key := "msg:" + msg.ID.Hex()
	if err := mc.redis.Set(ctx, key, data, 24*time.Hour); err != nil {
		return err
	}
	if !msg.ReceiverID.IsZero() {
		return mc.AddPendingDirectMessage(ctx, msg.ReceiverID.Hex(), msg.ID.Hex())
	}
	if !msg.GroupID.IsZero() {
		return mc.AddPendingGroupMessage(ctx, msg.GroupID.Hex(), msg.ID.Hex())
	}
	return nil
}

func (mc *MessageCache) Get(ctx context.Context, msgID string) (*models.Message, error) {
	data, err := mc.redis.Get(ctx, "msg:"+msgID)
	if err != nil {
		return nil, err
	}
	var m models.Message
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (mc *MessageCache) AddPendingDirectMessage(ctx context.Context, userID, msgID string) error {
	return mc.redis.SAdd(ctx, "pending:direct:"+userID, msgID).Err()
}

func (mc *MessageCache) GetPendingDirectMessages(ctx context.Context, userID string) ([]string, error) {
	return mc.redis.SMembers(ctx, "pending:direct:"+userID).Result()
}

func (mc *MessageCache) RemovePendingDirectMessage(ctx context.Context, userID, msgID string) error {
	return mc.redis.SRem(ctx, "pending:direct:"+userID, msgID).Err()
}

func (mc *MessageCache) AddPendingGroupMessage(ctx context.Context, groupID, msgID string) error {
	return mc.redis.SAdd(ctx, "pending:group:"+groupID, msgID).Err()
}

func (mc *MessageCache) GetPendingGroupMessages(ctx context.Context, groupID string) ([]string, error) {
	return mc.redis.SMembers(ctx, "pending:group:"+groupID).Result()
}

func (mc *MessageCache) RemovePendingGroupMessage(ctx context.Context, groupID, msgID string) error {
	return mc.redis.SRem(ctx, "pending:group:"+groupID, msgID).Err()
}

// ServeWs handles new websocket connections
func ServeWs(c *gin.Context, hub *Hub) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// TODO: restrict allowed origins
			return true
		},
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	userID, err := utils.GetUserIDFromContext(c)
	if err != nil || userID.IsZero() {
		log.Printf("Unauthorized WS attempt")
		conn.Close()
		return
	}

	groups, err := hub.groupRepo.GetUserGroups(c.Request.Context(), userID)
	if err != nil {
		log.Printf("Error fetching groups: %v", err)
	}
	listeners := make(map[string]bool)
	for _, g := range groups {
		listeners[g.ID.Hex()] = true
	}

	client := &Client{
		userID:    userID.Hex(),
		conn:      conn,
		send:      make(chan []byte, 256),
		lastSeen:  time.Now(),
		listeners: listeners,
	}
	hub.register <- client
	go client.writePump()
	go client.readPump(hub)
}

// readPump pumps messages from the websocket connection to the Hub
func (c *Client) readPump(h *Hub) {
	const (
		pongWait   = 60 * time.Second
		maxMsgSize = 512
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
			var e models.TypingEvent
			if err := json.Unmarshal(env.Payload, &e); err == nil && e.ConversationID != "" {
				h.typingEvents <- models.TypingEvent{UserID: c.userID, ConversationID: e.ConversationID, IsTyping: e.IsTyping, Timestamp: time.Now().Unix()}
			}
		case "message":
			var m models.Message
			if err := json.Unmarshal(env.Payload, &m); err == nil && m.Content != "" && m.SenderID.Hex() == c.userID {
				h.Broadcast <- m
			}
		case "presence":
			c.setLastSeen(time.Now())
		default:
			log.Printf("Unknown type: %s", env.Type)
		}
	}
}

// writePump pumps messages from the Hub to the websocket connection
func (c *Client) writePump() {
	const pingPeriod = (60 * time.Second * 9) / 10
	ticker := time.NewTicker(pingPeriod)
	defer func() { ticker.Stop(); c.conn.Close() }()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil { return }
			w.Write(msg)
			if err := w.Close(); err != nil { return }
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
