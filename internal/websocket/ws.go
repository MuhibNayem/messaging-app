package websocket

import (
	"context"
	"encoding/json"
	"fmt"
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
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	Status    string // Add this line
}

// MessageUpdater defines the interface for updating message statuses
type MessageUpdater interface {
	MarkMessagesAsDelivered(ctx context.Context, userID primitive.ObjectID, messageIDs []primitive.ObjectID) error
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	userClients  map[string]map[*Client]bool
	groupClients map[string]map[*Client]bool

	groupRepo      *repositories.GroupRepository
	feedRepo       *repositories.FeedRepository
	userRepo       *repositories.UserRepository
	friendshipRepo *repositories.FriendshipRepository // New
	messageRepo    *repositories.MessageRepository
	redisClient    *redis.ClusterClient
	messageCache   *MessageCache

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

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex

	messageUpdater MessageUpdater
}

// NewHub creates a new Hub and starts its goroutines
func NewHub(redisClient *redis.ClusterClient, groupRepo *repositories.GroupRepository, feedRepo *repositories.FeedRepository, userRepo *repositories.UserRepository, friendshipRepo *repositories.FriendshipRepository, messageRepo *repositories.MessageRepository, messageUpdater MessageUpdater) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Hub{
		userClients:            make(map[string]map[*Client]bool),
		groupClients:           make(map[string]map[*Client]bool),
		groupRepo:              groupRepo,
		feedRepo:               feedRepo,
		userRepo:               userRepo,
		friendshipRepo:         friendshipRepo, // Initialize
		messageRepo:            messageRepo,
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
		ctx:                    ctx,
		cancel:                 cancel,
		messageUpdater:         messageUpdater,
	}
	go h.run()
	go h.subscribeToRedis()
	go h.cleanupStaleConnections()
	return h
}

func (h *Hub) broadcastToParticipants(messageID primitive.ObjectID, wsEvent models.WebSocketEvent) {
	msg, err := h.messageRepo.GetMessageByID(h.ctx, messageID)
	if err != nil {
		log.Printf("Error getting message %s for event broadcasting: %v", messageID.Hex(), err)
		return
	}

	var participantIDs []string
	if !msg.GroupID.IsZero() {
		// Group message: get all members of the group
		group, err := h.groupRepo.GetGroup(h.ctx, msg.GroupID)
		if err != nil {
			log.Printf("Error getting group %s for event broadcasting: %v", msg.GroupID.Hex(), err)
			return
		}
		for _, memberID := range group.Members {
			participantIDs = append(participantIDs, memberID.Hex())
		}
	} else {
		// Direct message: sender and receiver
		participantIDs = append(participantIDs, msg.SenderID.Hex(), msg.ReceiverID.Hex())
	}

	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for targeted broadcast: %v", err)
		return
	}

	for _, userID := range participantIDs {
		h.sendToUser(userID, wsEventJSON)
	}
	log.Printf("Broadcasted %s event for message %s to %d participants", wsEvent.Type, messageID.Hex(), len(participantIDs))
}

func (h *Hub) run() {
	for {
		select {
		case <-h.ctx.Done():
			return

		case c := <-h.register:
			h.addClient(c)
			go h.sendCachedMessages(c)

			// Set presence in Redis
			presenceData, _ := json.Marshal(map[string]interface{}{"status": "online", "last_seen": time.Now().Unix()})
			h.redisClient.Set(h.ctx, "presence:"+c.userID, presenceData, 24*time.Hour)

			// 1. Fetch friends to send THEIR presence to the new client
			//    and to send the new client's presence to THEM.
			friends, err := h.friendshipRepo.GetFriends(h.ctx, func() primitive.ObjectID {
				oid, _ := primitive.ObjectIDFromHex(c.userID)
				return oid
			}())

			if err != nil {
				log.Printf("Error getting friends for presence: %v", err)
			}

			// Prepare presence event for the new user
			myPresenceEvent := models.WebSocketEvent{
				Type: "presence_update",
				Data: json.RawMessage(fmt.Sprintf(`{"user_id": "%s", "status": "online", "last_seen": %d}`, c.userID, time.Now().Unix())),
			}
			myPresenceNumBytes, _ := json.Marshal(myPresenceEvent)

			// List of friend IDs to notify
			friendIDs := make([]string, 0)
			if err == nil {
				for _, f := range friends {
					friendIDs = append(friendIDs, f.ID.Hex())

					// Check if friend is online
					h.mu.RLock()
					_, isOnline := h.userClients[f.ID.Hex()]
					h.mu.RUnlock()

					if isOnline {
						// Send friend's status to me
						friendPresence := models.WebSocketEvent{
							Type: "presence_update",
							Data: json.RawMessage(fmt.Sprintf(`{"user_id": "%s", "status": "online", "last_seen": %d}`, f.ID.Hex(), time.Now().Unix())),
						}
						friendPresenceBytes, _ := json.Marshal(friendPresence)
						c.send <- friendPresenceBytes
					}
				}
			}

			// 2. Broadcast my presence ONLY to my friends
			for _, fid := range friendIDs {
				h.sendToUser(fid, myPresenceNumBytes)
			}
			// Also send to self to confirm connection (optional but good for consistency)
			c.send <- myPresenceNumBytes

		case c := <-h.unregister:
			h.removeClient(c)

			// Set presence in Redis
			presenceData, _ := json.Marshal(map[string]interface{}{"status": "offline", "last_seen": time.Now().Unix()})
			h.redisClient.Set(h.ctx, "presence:"+c.userID, presenceData, 24*time.Hour)

			// Broadcast offline status ONLY to friends
			friends, err := h.friendshipRepo.GetFriends(h.ctx, func() primitive.ObjectID {
				oid, _ := primitive.ObjectIDFromHex(c.userID)
				return oid
			}())

			if err == nil {
				offlineEvent := models.WebSocketEvent{
					Type: "presence_update",
					Data: json.RawMessage(fmt.Sprintf(`{"user_id": "%s", "status": "offline", "last_seen": %d}`, c.userID, time.Now().Unix())),
				}
				offlineEventBytes, _ := json.Marshal(offlineEvent)

				for _, f := range friends {
					h.sendToUser(f.ID.Hex(), offlineEventBytes)
				}
			}

		case event := <-h.FeedEvents:
			switch event.Type {
			case "PostCreated":
				var post models.Post
				if err := json.Unmarshal(event.Data, &post); err != nil {
					log.Printf("Error unmarshaling PostCreated data: %v", err)
					continue
				}

				// Handle privacy-aware broadcasting
				switch post.Privacy {
				case models.PrivacySettingPublic:
					// Broadcast to all connected clients
					h.broadcastToAllUsers(event)
				case models.PrivacySettingOnlyMe:
					// Send only to the post author
					h.sendToUser(post.UserID.Hex(), event.Data)
				case models.PrivacySettingFriends:
					// Send to author
					h.sendToUser(post.UserID.Hex(), event.Data)

					// Get friends of the post author
					// Note: Using background context here, might consider passing a context if available
					friends, err := h.friendshipRepo.GetFriends(context.Background(), post.UserID)
					if err != nil {
						log.Printf("Error getting friends for post broadcast: %v", err)
						continue
					}

					// Send to each friend
					for _, friend := range friends {
						h.sendToUser(friend.ID.Hex(), event.Data)
					}
				default:
					// Default to author only for unknown privacy settings (fail safe)
					h.sendToUser(post.UserID.Hex(), event.Data)
				}

				log.Printf("Broadcasted PostCreated event for post %s (Privacy: %s)", post.ID.Hex(), post.Privacy)

			case "PostUpdated":
				var post models.Post
				if err := json.Unmarshal(event.Data, &post); err != nil {
					log.Printf("Error unmarshaling PostUpdated data: %v", err)
					continue
				}

				// Handle privacy-aware broadcasting
				switch post.Privacy {
				case models.PrivacySettingPublic:
					h.broadcastToAllUsers(event)
				case models.PrivacySettingOnlyMe:
					h.sendToUser(post.UserID.Hex(), event.Data)
				case models.PrivacySettingFriends:
					h.sendToUser(post.UserID.Hex(), event.Data)
					friends, err := h.friendshipRepo.GetFriends(context.Background(), post.UserID)
					if err != nil {
						log.Printf("Error getting friends for post update broadcast: %v", err)
						continue
					}
					for _, friend := range friends {
						h.sendToUser(friend.ID.Hex(), event.Data)
					}
				default:
					h.sendToUser(post.UserID.Hex(), event.Data)
				}
				log.Printf("Broadcasted PostUpdated event for post %s", post.ID.Hex())

			case "PostDeleted":
				var post models.Post
				if err := json.Unmarshal(event.Data, &post); err != nil {
					log.Printf("Error unmarshaling PostDeleted data: %v", err)
					continue
				}
				// For deletion, we can broadcast to all, as it just tells clients to remove the ID.
				// Or we can be precise. Let's be precise to avoid noise.
				switch post.Privacy {
				case models.PrivacySettingPublic:
					h.broadcastToAllUsers(event)
				case models.PrivacySettingOnlyMe:
					h.sendToUser(post.UserID.Hex(), event.Data)
				case models.PrivacySettingFriends:
					h.sendToUser(post.UserID.Hex(), event.Data)
					friends, err := h.friendshipRepo.GetFriends(context.Background(), post.UserID)
					if err != nil {
						log.Printf("Error getting friends for post delete broadcast: %v", err)
						continue
					}
					for _, friend := range friends {
						h.sendToUser(friend.ID.Hex(), event.Data)
					}
				default:
					h.sendToUser(post.UserID.Hex(), event.Data)
				}
				log.Printf("Broadcasted PostDeleted event for post %s", post.ID.Hex())

			case "CommentCreated":
				var comment models.Comment
				if err := json.Unmarshal(event.Data, &comment); err != nil {
					log.Printf("Error unmarshaling CommentCreated data: %v", err)
					continue
				}
				// Fetch the post to get the owner's ID
				post, err := h.feedRepo.GetPostByID(context.Background(), comment.PostID)
				if err != nil {
					log.Printf("Error getting post %s for comment %s: %v", comment.PostID.Hex(), comment.ID.Hex(), err)
					continue
				}
				h.sendToUser(post.UserID.Hex(), event.Data) // Send to post owner

				// Also broadcast to others who can view the post (same logic as PostCreated)
				switch post.Privacy {
				case models.PrivacySettingPublic:
					h.broadcastToAllUsers(event)
				case models.PrivacySettingFriends:
					friends, err := h.friendshipRepo.GetFriends(context.Background(), post.UserID)
					if err == nil {
						for _, friend := range friends {
							h.sendToUser(friend.ID.Hex(), event.Data)
						}
					}
				}

				log.Printf("Broadcasted CommentCreated event for comment %s on post %s", comment.ID.Hex(), comment.PostID.Hex())

			case "ReplyCreated":
				var reply models.Reply
				if err := json.Unmarshal(event.Data, &reply); err != nil {
					log.Printf("Error unmarshaling ReplyCreated data: %v", err)
					continue
				}
				// Fetch the comment to get the owner's ID
				comment, err := h.feedRepo.GetCommentByID(context.Background(), reply.CommentID)
				if err != nil {
					log.Printf("Error getting comment %s for reply %s: %v", reply.CommentID.Hex(), reply.ID.Hex(), err)
					continue
				}
				h.sendToUser(comment.UserID.Hex(), event.Data) // Send to comment owner

				// Fetch post to check privacy for broader broadcast
				post, err := h.feedRepo.GetPostByID(context.Background(), comment.PostID)
				if err != nil {
					log.Printf("Error getting post %s for reply broadcast: %v", comment.PostID.Hex(), err)
				} else {
					h.sendToUser(post.UserID.Hex(), event.Data) // Send to post owner as well

					switch post.Privacy {
					case models.PrivacySettingPublic:
						h.broadcastToAllUsers(event)
					case models.PrivacySettingFriends:
						friends, err := h.friendshipRepo.GetFriends(context.Background(), post.UserID)
						if err == nil {
							for _, friend := range friends {
								h.sendToUser(friend.ID.Hex(), event.Data)
							}
						}
					}
				}

				log.Printf("Broadcasted ReplyCreated event for reply %s on comment %s", reply.ID.Hex(), reply.CommentID.Hex())

			case "ReactionCreated":
				var reaction models.Reaction
				if err := json.Unmarshal(event.Data, &reaction); err != nil {
					log.Printf("Error unmarshaling ReactionCreated data: %v", err)
					continue
				}
				// Broadcast to all users for real-time update of reaction counts
				h.broadcastToAllUsers(event)
				log.Printf("Broadcasted ReactionCreated event for reaction %s on target %s (type: %s)", reaction.ID.Hex(), reaction.TargetID.Hex(), reaction.TargetType)

			case "ReactionDeleted": // Handle ReactionDeleted event
				var reaction models.Reaction
				if err := json.Unmarshal(event.Data, &reaction); err != nil {
					log.Printf("Error unmarshaling ReactionDeleted data: %v", err)
					continue
				}
				// Broadcast to all users for real-time update of reaction counts
				h.broadcastToAllUsers(event)
				log.Printf("Broadcasted ReactionDeleted event for reaction %s on target %s (type: %s)", reaction.ID.Hex(), reaction.TargetID.Hex(), reaction.TargetType)

			default:
				log.Printf("Received unknown WebSocket event type: %s, data: %s", event.Type, string(event.Data))
			}
		case notification := <-h.NotificationEvents:
			notificationJSON, err := json.Marshal(notification)
			if err != nil {
				log.Printf("Error marshaling notification for WebSocket: %v", err)
				continue
			}
			wsEvent := models.WebSocketEvent{
				Type: "NOTIFICATION_CREATED",
				Data: notificationJSON,
			}
			wsEventJSON, err := json.Marshal(wsEvent)
			if err != nil {
				log.Printf("Error marshaling WebSocketEvent for notification: %v", err)
				continue
			}
			h.sendToUser(notification.RecipientID.Hex(), wsEventJSON)
			log.Printf("Sent NOTIFICATION_CREATED event to user %s for notification %s", notification.RecipientID.Hex(), notification.ID.Hex())

		case ev := <-h.typingEvents:
			h.dispatchTypingEvent(ev)

		case m := <-h.Broadcast:
			h.dispatchMessage(m)

		case reactionEvent := <-h.ReactionEvents:
			reactionEventJSON, err := json.Marshal(reactionEvent)
			if err != nil {
				log.Printf("Error marshaling ReactionEvent for WebSocket: %v", err)
				continue
			}
			wsEvent := models.WebSocketEvent{
				Type: "MESSAGE_REACTION_UPDATE",
				Data: reactionEventJSON,
			}
			h.broadcastToParticipants(reactionEvent.MessageID, wsEvent)

		case readReceiptEvent := <-h.ReadReceiptEvents:
			readReceiptEventJSON, err := json.Marshal(readReceiptEvent)
			if err != nil {
				log.Printf("Error marshaling ReadReceiptEvent for WebSocket: %v", err)
				continue
			}
			wsEvent := models.WebSocketEvent{
				Type: "MESSAGE_READ_UPDATE",
				Data: readReceiptEventJSON,
			}
			wsEventJSON, err := json.Marshal(wsEvent)
			if err != nil {
				log.Printf("Error marshaling WebSocketEvent for read receipt: %v", err)
				continue
			}

			// Notify the reader that their action was processed
			h.sendToUser(readReceiptEvent.ReaderID.Hex(), wsEventJSON)

			// Notify the sender of the messages that they were read
			for _, msgID := range readReceiptEvent.MessageIDs {
				msg, err := h.messageRepo.GetMessageByID(h.ctx, msgID)
				if err != nil {
					log.Printf("Error getting message %s for read receipt: %v", msgID.Hex(), err)
					continue
				}
				// Avoid sending notification to self
				if msg.SenderID != readReceiptEvent.ReaderID {
					h.sendToUser(msg.SenderID.Hex(), wsEventJSON)
				}
			}

		case ev := <-h.MessageEditedEvents:
			h.broadcastToParticipants(ev.MessageID, models.WebSocketEvent{Type: "MESSAGE_EDITED_UPDATE", Data: json.RawMessage(fmt.Sprintf(`{"message_id": "%s", "new_content": "%s"}`, ev.MessageID.Hex(), ev.NewContent))})

		case conversationSeenEvent := <-h.ConversationSeenEvents:
			conversationSeenEventJSON, err := json.Marshal(conversationSeenEvent)
			if err != nil {
				log.Printf("Error marshaling ConversationSeenEvent for WebSocket: %v", err)
				continue
			}
			wsEvent := models.WebSocketEvent{
				Type: "CONVERSATION_SEEN_UPDATE",
				Data: conversationSeenEventJSON,
			}
			wsEventJSON, err := json.Marshal(wsEvent)
			if err != nil {
				log.Printf("Error marshaling WebSocketEvent for conversation seen: %v", err)
				continue
			}

			if conversationSeenEvent.IsGroup {
				group, err := h.groupRepo.GetGroup(h.ctx, conversationSeenEvent.ConversationID)
				if err != nil {
					log.Printf("Error getting group %s for conversation seen event: %v", conversationSeenEvent.ConversationID.Hex(), err)
					continue
				}
				for _, memberID := range group.Members {
					h.sendToUser(memberID.Hex(), wsEventJSON)
				}
			} else {
				h.sendToUser(conversationSeenEvent.UserID.Hex(), wsEventJSON)
				h.sendToUser(conversationSeenEvent.ConversationID.Hex(), wsEventJSON)
			}

		case dev := <-h.DeliveredEvents:
			// Mark messages as delivered in the database asynchronously
			go func(delivererID primitive.ObjectID, messageIDs []primitive.ObjectID) {
				// Create a new context as the hub's context might be cancelled or not appropriate for long running DB ops if we wanted strict timeouts
				// But context.Background() is safer for detached async ops
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				err := h.messageUpdater.MarkMessagesAsDelivered(ctx, delivererID, messageIDs)
				if err != nil {
					log.Printf("Error marking messages as delivered: %v", err)
				}
			}(dev.DelivererID, dev.MessageIDs)

			// Notify relevant clients about the delivery update
			deliveredEventJSON, err := json.Marshal(dev)
			if err != nil {
				log.Printf("Error marshaling DeliveredEvent for WebSocket: %v", err)
				continue
			}
			wsEvent := models.WebSocketEvent{
				Type: "MESSAGE_DELIVERED_UPDATE",
				Data: deliveredEventJSON,
			}
			// Find the message to get its conversation context for targeted broadcast
			if len(dev.MessageIDs) > 0 {
				msg, err := h.messageRepo.GetMessageByID(h.ctx, dev.MessageIDs[0])
				if err != nil {
					log.Printf("Error getting message %s for delivered event broadcast: %v", dev.MessageIDs[0].Hex(), err)
					continue
				}
				// Determine conversation type and ID
				var clients []*Client
				if !msg.GroupID.IsZero() {
					clients = h.getClientsByGroup(msg.GroupID.Hex())
				} else if !msg.ReceiverID.IsZero() {
					// For direct messages, send to sender and receiver
					clients = append(h.getClientsByUser(msg.SenderID.Hex()), h.getClientsByUser(msg.ReceiverID.Hex())...)
				}

				wsEventJSON, err := json.Marshal(wsEvent)
				if err != nil {
					log.Printf("Error marshaling WebSocketEvent for delivered update: %v", err)
					continue
				}

				for _, c := range clients {
					// Don't send delivered update to the deliverer themselves
					if c.userID == dev.DelivererID.Hex() {
						continue
					}
					select {
					case c.send <- wsEventJSON:
						log.Printf("Sent MESSAGE_DELIVERED_UPDATE for message %s to user %s", dev.MessageIDs[0].Hex(), c.userID)
					default:
						h.removeClient(c)
					}
				}
			}
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

// sendToUser sends a message to all active WebSocket connections for a specific userID.
func (h *Hub) sendToUser(userID string, message []byte) {
	if clients, ok := h.userClients[userID]; ok {
		for client := range clients {
			select {
			case client.send <- message:
			default:
				close(client.send)
				// The client is already removed from userClients by removeUserClient
				// No need to delete from h.clients as it's not directly managed here
				h.removeUserClient(client.userID, client)
			}
		}
	}
}

// broadcastToAllUsers sends a message to all currently connected WebSocket clients.
func (h *Hub) broadcastToAllUsers(event models.WebSocketEvent) {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for broadcast: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for userID := range h.userClients {
		for client := range h.userClients[userID] {
			select {
			case client.send <- eventBytes:
				// Message sent successfully
			default:
				// Client's send channel is full, remove client
				close(client.send)
				h.removeUserClient(client.userID, client)
			}
		}
	}
}

func (h *Hub) dispatchMessage(msg models.Message) {
	// direct
	if !msg.ReceiverID.IsZero() {
		clients := h.getClientsByUser(msg.ReceiverID.Hex())
		h.sendToClients(clients, msg)

		// If user is offline (no active clients), queue the message for delivery upon reconnection
		if len(clients) == 0 {
			if err := h.messageCache.AddPendingDirectMessage(h.ctx, msg.ReceiverID.Hex(), msg.ID.Hex()); err != nil {
				log.Printf("Failed to queue pending direct message for %s: %v", msg.ReceiverID.Hex(), err)
			}
		}
		return
	}
	// group
	if !msg.GroupID.IsZero() {
		h.sendToClients(h.getClientsByGroup(msg.GroupID.Hex()), msg)

		// Queue pending messages asynchronously to avoid blocking the hub loop
		// queuePendingForGroup handles its own locking safely
		go h.queuePendingForGroup(msg)
	}
}

func (h *Hub) sendToClients(clients []*Client, msg models.Message) {
	msgData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	wsEvent := models.WebSocketEvent{
		Type: "MESSAGE_CREATED",
		Data: msgData,
	}

	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for message: %v", err)
		return
	}

	for _, c := range clients {
		select {
		case c.send <- wsEventJSON:
			c.setLastSeen(time.Now())
			wsMessagesSent.WithLabelValues(msg.ContentType).Inc()

			// Send a delivered event to the hub for processing
			delivererObjectID, err := primitive.ObjectIDFromHex(c.userID)
			if err != nil {
				log.Printf("Error converting deliverer ID to ObjectID: %v", err)
				return
			}
			h.DeliveredEvents <- models.DeliveredEvent{
				MessageIDs:  []primitive.ObjectID{msg.ID},
				DelivererID: delivererObjectID,
				Timestamp:   time.Now(),
			}
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
	conversationType := ""
	conversationID := ev.ConversationID

	if len(conversationID) > 5 && conversationID[:5] == "user-" {
		conversationType = "user"
		conversationID = conversationID[5:] // Extract actual user ID
	} else if len(conversationID) > 6 && conversationID[:6] == "group-" {
		conversationType = "group"
		conversationID = conversationID[6:] // Extract actual group ID
	} else {
		log.Printf("Invalid conversation ID format for typing event: %s", ev.ConversationID)
		return
	}

	var clients []*Client
	if conversationType == "user" {
		clients = h.getClientsByUser(conversationID)
	} else if conversationType == "group" {
		clients = h.getClientsByGroup(conversationID)
	}

	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("Error marshaling typing event: %v", err)
		return
	}
	// Create a WebSocketEvent to send to the client
	wsEvent := models.WebSocketEvent{
		Type: "TYPING",
		Data: data, // The marshaled TypingEvent is the data payload
	}

	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for typing: %v", err)
		return
	}

	log.Printf("Dispatching typing event: %+v to %d clients", ev, len(clients))
	for _, c := range clients {
		if c.userID == ev.UserID {
			continue
		}
		select {
		case c.send <- wsEventJSON:
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
			var typingData struct {
				ConversationID string `json:"conversation_id"`
				IsTyping       bool   `json:"isTyping"`
			}
			if err := json.Unmarshal(env.Payload, &typingData); err != nil {
				log.Printf("Error unmarshaling typing data: %v", err)
				return
			}
			h.typingEvents <- models.TypingEvent{UserID: c.userID, ConversationID: typingData.ConversationID, IsTyping: typingData.IsTyping, Timestamp: time.Now().Unix()}
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
			if err != nil {
				return
			}
			w.Write(msg)
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
