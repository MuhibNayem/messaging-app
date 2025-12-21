package websocket

import (
	"context"
	"sync"

	"gitlab.com/spydotech-group/shared-entity/models"
	"gitlab.com/spydotech-group/shared-entity/redis"
	"messaging-app/internal/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MessageUpdater defines the interface for updating message statuses.
type MessageUpdater interface {
	MarkMessagesAsDelivered(ctx context.Context, userID primitive.ObjectID, conversationID string, messageIDs []string) error
}

// Hub maintains the set of active clients and orchestrates WebSocket events.
type Hub struct {
	userClients  map[string]map[*Client]bool
	groupClients map[string]map[*Client]bool

	groupRepo            *repositories.GroupRepository
	feedRepo             *repositories.FeedRepository
	userRepo             *repositories.UserRepository
	friendshipRepo       *repositories.FriendshipRepository
	messageRepo          *repositories.MessageRepository
	messageCassandraRepo *repositories.MessageCassandraRepository
	redisClient          *redis.ClusterClient
	messageCache         *MessageCache

	register               chan *Client
	unregister             chan *Client
	Broadcast              chan models.Message
	FeedEvents             chan models.WebSocketEvent
	NotificationEvents     chan models.Notification
	typingEvents           chan models.TypingEvent
	ReactionEvents         chan models.ReactionEvent
	ReadReceiptEvents      chan models.ReadReceiptEvent
	MessageEditedEvents    chan models.MessageEditedEvent
	DeliveredEvents        chan models.DeliveredEvent
	ConversationSeenEvents chan models.ConversationSeenEvent
	EventRSVPEvents        chan models.EventRSVPEvent // Channel for event RSVP updates
	CallSignal             chan models.CallSignalEvent

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex

	messageUpdater MessageUpdater
}

// NewHub creates a new Hub and starts its background goroutines.
func NewHub(
	redisClient *redis.ClusterClient,
	groupRepo *repositories.GroupRepository,
	feedRepo *repositories.FeedRepository,
	userRepo *repositories.UserRepository,
	friendshipRepo *repositories.FriendshipRepository,
	messageRepo *repositories.MessageRepository,
	messageCassandraRepo *repositories.MessageCassandraRepository,
	messageUpdater MessageUpdater,
) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Hub{
		userClients:            make(map[string]map[*Client]bool),
		groupClients:           make(map[string]map[*Client]bool),
		groupRepo:              groupRepo,
		feedRepo:               feedRepo,
		userRepo:               userRepo,
		friendshipRepo:         friendshipRepo,
		messageRepo:            messageRepo,
		messageCassandraRepo:   messageCassandraRepo,
		redisClient:            redisClient,
		messageCache:           NewMessageCache(redisClient),
		register:               make(chan *Client),
		unregister:             make(chan *Client),
		Broadcast:              make(chan models.Message, 10000),
		FeedEvents:             make(chan models.WebSocketEvent, 10000),
		NotificationEvents:     make(chan models.Notification, 10000),
		typingEvents:           make(chan models.TypingEvent, 1000),
		ReactionEvents:         make(chan models.ReactionEvent, 10000),
		ReadReceiptEvents:      make(chan models.ReadReceiptEvent, 10000),
		MessageEditedEvents:    make(chan models.MessageEditedEvent, 10000),
		DeliveredEvents:        make(chan models.DeliveredEvent, 10000),
		ConversationSeenEvents: make(chan models.ConversationSeenEvent, 10000),
		EventRSVPEvents:        make(chan models.EventRSVPEvent, 10000),
		CallSignal:             make(chan models.CallSignalEvent, 10000),
		ctx:                    ctx,
		cancel:                 cancel,
		messageUpdater:         messageUpdater,
	}

	go h.run()
	go h.subscribeToRedis()
	go h.cleanupStaleConnections()

	return h
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

	if conns, ok := h.userClients[c.userID]; ok {
		if _, exists := conns[c]; exists {
			delete(conns, c)
			if len(conns) == 0 {
				delete(h.userClients, c.userID)
			}
		}
	}

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

func (h *Hub) removeUserClient(userID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.userClients[userID] != nil {
		delete(h.userClients[userID], client)
		if len(h.userClients[userID]) == 0 {
			delete(h.userClients, userID)
		}
	}
}
