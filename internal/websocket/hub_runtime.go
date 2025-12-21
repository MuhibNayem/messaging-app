package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gitlab.com/spydotech-group/shared-entity/models"
	"gitlab.com/spydotech-group/shared-entity/utils"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Hub) broadcastToParticipants(messageID primitive.ObjectID, wsEvent models.WebSocketEvent) {
	msg, err := h.messageRepo.GetMessageByID(h.ctx, messageID)
	if err != nil {
		log.Printf("Error getting message %s for event broadcasting: %v", messageID.Hex(), err)
		return
	}

	var participantIDs []string
	if !msg.GroupID.IsZero() {
		group, err := h.groupRepo.GetGroup(h.ctx, msg.GroupID)
		if err != nil {
			log.Printf("Error getting group %s for event broadcasting: %v", msg.GroupID.Hex(), err)
			return
		}
		for _, memberID := range group.Members {
			participantIDs = append(participantIDs, memberID.Hex())
		}
	} else {
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
			h.handleRegister(c)
		case c := <-h.unregister:
			h.handleUnregister(c)
		case event := <-h.FeedEvents:
			go h.handleFeedEvent(event)
		case notification := <-h.NotificationEvents:
			h.handleNotification(notification)
		case ev := <-h.typingEvents:
			h.dispatchTypingEvent(ev)
		case m := <-h.Broadcast:
			h.dispatchMessage(m)
		case reactionEvent := <-h.ReactionEvents:
			go h.handleReactionEvent(reactionEvent)
		case readReceiptEvent := <-h.ReadReceiptEvents:
			go h.handleReadReceiptEvent(readReceiptEvent)
		case ev := <-h.MessageEditedEvents:
			go h.handleMessageEditedEvent(ev)
		case conversationSeenEvent := <-h.ConversationSeenEvents:
			go h.handleConversationSeenEvent(conversationSeenEvent)
		case dev := <-h.DeliveredEvents:
			h.handleDeliveredEvent(dev)
		case signal := <-h.CallSignal:
			h.handleCallSignal(signal)
		case rsvpEvent := <-h.EventRSVPEvents:
			go h.handleEventRSVPEvent(rsvpEvent)
		}
	}
}

func (h *Hub) handleRegister(c *Client) {
	h.addClient(c)
	go h.sendCachedMessages(c)

	go func(client *Client) {
		presenceData, _ := json.Marshal(map[string]interface{}{"status": "online", "last_seen": time.Now().Unix()})
		h.redisClient.Set(h.ctx, "presence:"+client.userID, presenceData, 24*time.Hour)

		userOID, _ := primitive.ObjectIDFromHex(client.userID)
		friends, err := h.friendshipRepo.GetFriends(h.ctx, userOID)
		if err != nil {
			log.Printf("Error getting friends for presence: %v", err)
		}

		myPresenceEvent := models.WebSocketEvent{
			Type: "presence_update",
			Data: json.RawMessage(fmt.Sprintf(`{"user_id": "%s", "status": "online", "last_seen": %d}`, client.userID, time.Now().Unix())),
		}
		myPresenceNumBytes, _ := json.Marshal(myPresenceEvent)

		notifyUserIDs := make(map[string]bool)
		for _, f := range friends {
			notifyUserIDs[f.ID.Hex()] = true
			h.sendFriendPresenceToClient(client, f.ID.Hex())
		}

		var marketplacePartners []primitive.ObjectID
		var mpErr error
		if h.messageCassandraRepo != nil {
			marketplacePartners, mpErr = h.messageCassandraRepo.GetMarketplacePartnerIDs(h.ctx, userOID)
		}
		if (mpErr != nil || len(marketplacePartners) == 0) && h.messageRepo != nil {
			marketplacePartners, mpErr = h.messageRepo.GetMarketplacePartnerIDs(h.ctx, userOID)
		}
		if mpErr != nil {
			log.Printf("Error getting marketplace partners for presence: %v", mpErr)
		} else {
			for _, partnerID := range marketplacePartners {
				partnerHex := partnerID.Hex()
				notifyUserIDs[partnerHex] = true
				h.sendFriendPresenceToClient(client, partnerHex)
			}
		}

		for userID := range notifyUserIDs {
			h.sendToUser(userID, myPresenceNumBytes)
		}

		h.mu.RLock()
		if clients, ok := h.userClients[client.userID]; ok {
			if _, exists := clients[client]; exists {
				client.send <- myPresenceNumBytes
			}
		}
		h.mu.RUnlock()
	}(c)
}

func (h *Hub) handleUnregister(c *Client) {
	h.removeClient(c)

	go func(userID string) {
		presenceData, _ := json.Marshal(map[string]interface{}{"status": "offline", "last_seen": time.Now().Unix()})
		h.redisClient.Set(h.ctx, "presence:"+userID, presenceData, 24*time.Hour)

		userOID, _ := primitive.ObjectIDFromHex(userID)
		notifyUserIDs := make(map[string]bool)

		friends, err := h.friendshipRepo.GetFriends(h.ctx, userOID)
		if err == nil {
			for _, f := range friends {
				notifyUserIDs[f.ID.Hex()] = true
			}
		}

		marketplacePartners, mpErr := h.messageRepo.GetMarketplacePartnerIDs(h.ctx, userOID)
		if mpErr == nil {
			for _, partnerID := range marketplacePartners {
				notifyUserIDs[partnerID.Hex()] = true
			}
		}

		offlineEvent := models.WebSocketEvent{
			Type: "presence_update",
			Data: json.RawMessage(fmt.Sprintf(`{"user_id": "%s", "status": "offline", "last_seen": %d}`, userID, time.Now().Unix())),
		}
		offlineEventBytes, _ := json.Marshal(offlineEvent)

		for uid := range notifyUserIDs {
			h.sendToUser(uid, offlineEventBytes)
		}
	}(c.userID)
}

func (h *Hub) sendFriendPresenceToClient(client *Client, friendID string) {
	h.mu.RLock()
	_, isOnline := h.userClients[friendID]
	h.mu.RUnlock()

	if !isOnline {
		return
	}

	friendPresence := models.WebSocketEvent{
		Type: "presence_update",
		Data: json.RawMessage(fmt.Sprintf(`{"user_id": "%s", "status": "online", "last_seen": %d}`, friendID, time.Now().Unix())),
	}
	friendPresenceBytes, _ := json.Marshal(friendPresence)

	h.mu.RLock()
	if clients, ok := h.userClients[client.userID]; ok {
		if _, exists := clients[client]; exists {
			client.send <- friendPresenceBytes
		}
	}
	h.mu.RUnlock()
}

func (h *Hub) sendToUser(userID string, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.userClients[userID]; ok {
		for client := range clients {
			select {
			case client.send <- message:
			default:
				close(client.send)
				go h.removeUserClient(client.userID, client)
			}
		}
	}
}

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
			default:
				close(client.send)
				go h.removeUserClient(client.userID, client)
			}
		}
	}
}

func (h *Hub) dispatchMessage(msg models.Message) {
	if !msg.ReceiverID.IsZero() {
		log.Printf("[DEBUG] Dispatching direct message ID: %s to Receiver: %s", msg.ID.Hex(), msg.ReceiverID.Hex())
		receiverClients := h.getClientsByUser(msg.ReceiverID.Hex())
		log.Printf("[DEBUG] Found %d clients for Receiver: %s", len(receiverClients), msg.ReceiverID.Hex())

		h.sendToClients(receiverClients, msg)

		if len(receiverClients) == 0 {
			log.Printf("[DEBUG] Receiver %s is offline, queuing message", msg.ReceiverID.Hex())
			if err := h.messageCache.AddPendingDirectMessage(h.ctx, msg.ReceiverID.Hex(), msg.ID.Hex()); err != nil {
				log.Printf("Failed to queue pending direct message for %s: %v", msg.ReceiverID.Hex(), err)
			}
		}
		return
	}

	if !msg.GroupID.IsZero() {
		h.sendToClients(h.getClientsByGroup(msg.GroupID.Hex()), msg)
		go h.queuePendingForGroup(msg)
	}
}

func (h *Hub) sendToClients(clients []*Client, msg models.Message) {
	var msgToMarshal interface{} = msg
	if msg.GroupID.IsZero() {
		type shadowedMessage struct {
			models.Message
			GroupID *string `json:"group_id,omitempty"`
		}
		msgToMarshal = shadowedMessage{
			Message: msg,
			GroupID: nil,
		}
	}

	msgData, err := json.Marshal(msgToMarshal)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	eventType := "MESSAGE_CREATED"
	if msg.IsMarketplace {
		eventType = "MARKETPLACE_MESSAGE_CREATED"
	}

	if msg.ContentType == "deleted" || msg.ContentType == models.ContentTypeDeleted {
		if msg.IsMarketplace {
			eventType = "MARKETPLACE_MESSAGE_DELETED"
		} else {
			eventType = "MESSAGE_DELETED"
		}
	}

	wsEvent := models.WebSocketEvent{
		Type: eventType,
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
			go h.notifyDelivery(c, msg)
		default:
			h.removeClient(c)
		}
	}
}

func (h *Hub) notifyDelivery(c *Client, message models.Message) {
	delivererObjectID, err := primitive.ObjectIDFromHex(c.userID)
	if err != nil {
		log.Printf("Error converting deliverer ID to ObjectID: %v", err)
		return
	}

	var conversationID string
	if !message.GroupID.IsZero() {
		conversationID = fmt.Sprintf("group_%s", message.GroupID.Hex())
	} else {
		conversationID = utils.GetConversationID(message.SenderID, message.ReceiverID)
	}

	msgIDStr := message.StringID
	if msgIDStr == "" {
		msgIDStr = message.ID.Hex()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.messageUpdater.MarkMessagesAsDelivered(ctx, delivererObjectID, conversationID, []string{msgIDStr}); err != nil {
		log.Printf("Error marking message %s as delivered to %s: %v", msgIDStr, c.userID, err)
	}

	deliveredEvent := models.DeliveredEvent{
		MessageIDs:  []primitive.ObjectID{message.ID},
		DelivererID: delivererObjectID,
		Timestamp:   time.Now(),
	}
	eventBytes, err := json.Marshal(deliveredEvent)
	if err != nil {
		log.Printf("Error marshaling delivered event: %v", err)
		return
	}
	wsEvent := models.WebSocketEvent{
		Type: "MESSAGE_DELIVERED_UPDATE",
		Data: eventBytes,
	}
	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for delivered update: %v", err)
		return
	}
	h.sendToUser(message.SenderID.Hex(), wsEventJSON)
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

func (h *Hub) sendPendingMessages(client *Client, msgIDs []string, msgType string) {
	ctx := h.ctx

	for _, id := range msgIDs {
		msg, err := h.messageCache.Get(ctx, id)
		if err != nil {
			log.Printf("Error retrieving message %s: %v", id, err)
			continue
		}

		if msgType == "direct" {
			if msg.ReceiverID.Hex() != client.userID {
				continue
			}
		} else {
			if !client.listeners[msg.GroupID.Hex()] {
				continue
			}
		}

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
		conversationID = conversationID[5:]
	} else if len(conversationID) > 6 && conversationID[:6] == "group-" {
		conversationType = "group"
		conversationID = conversationID[6:]
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
	wsEvent := models.WebSocketEvent{
		Type: "TYPING",
		Data: data,
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

func (h *Hub) handleFeedEvent(event models.WebSocketEvent) {
	switch event.Type {
	case "PostCreated":
		var post models.Post
		if err := json.Unmarshal(event.Data, &post); err != nil {
			log.Printf("Error unmarshaling PostCreated data: %v", err)
			return
		}
		switch post.Privacy {
		case models.PrivacySettingPublic:
			h.broadcastToAllUsers(event)
		case models.PrivacySettingOnlyMe:
			h.sendToUser(post.UserID.Hex(), event.Data)
		case models.PrivacySettingFriends:
			h.sendToUser(post.UserID.Hex(), event.Data)
			friends, err := h.friendshipRepo.GetFriends(context.Background(), post.UserID)
			if err != nil {
				log.Printf("Error getting friends for post broadcast: %v", err)
				return
			}
			for _, friend := range friends {
				h.sendToUser(friend.ID.Hex(), event.Data)
			}
		default:
			h.sendToUser(post.UserID.Hex(), event.Data)
		}
		log.Printf("Broadcasted PostCreated event for post %s (Privacy: %s)", post.ID.Hex(), post.Privacy)

	case "GROUP_UPDATED", "GROUP_CREATED":
		var group models.GroupResponse
		if err := json.Unmarshal(event.Data, &group); err != nil {
			log.Printf("Error unmarshaling GROUP_UPDATED data: %v", err)
			return
		}
		eventBytes, err := json.Marshal(event)
		if err != nil {
			log.Printf("Error marshaling GROUP_UPDATED event wrapper: %v", err)
			return
		}
		for _, member := range group.Members {
			h.sendToUser(member.ID.Hex(), eventBytes)
		}
		log.Printf("Broadcasted GROUP_UPDATED event for group %s to %d members", group.ID.Hex(), len(group.Members))

	case "PostUpdated":
		var post models.Post
		if err := json.Unmarshal(event.Data, &post); err != nil {
			log.Printf("Error unmarshaling PostUpdated data: %v", err)
			return
		}
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
				return
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
			return
		}
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
				return
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
			return
		}
		post, err := h.feedRepo.GetPostByID(context.Background(), comment.PostID)
		if err != nil {
			log.Printf("Error getting post %s for comment %s: %v", comment.PostID.Hex(), comment.ID.Hex(), err)
			return
		}
		h.sendToUser(post.UserID.Hex(), event.Data)

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
			return
		}
		comment, err := h.feedRepo.GetCommentByID(context.Background(), reply.CommentID)
		if err != nil {
			log.Printf("Error getting comment %s for reply %s: %v", reply.CommentID.Hex(), reply.ID.Hex(), err)
			return
		}
		h.sendToUser(comment.UserID.Hex(), event.Data)

		post, err := h.feedRepo.GetPostByID(context.Background(), comment.PostID)
		if err != nil {
			log.Printf("Error getting post %s for reply broadcast: %v", comment.PostID.Hex(), err)
		} else {
			h.sendToUser(post.UserID.Hex(), event.Data)

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

	case "ReactionCreated", "ReactionDeleted":
		var reaction models.Reaction
		if err := json.Unmarshal(event.Data, &reaction); err != nil {
			log.Printf("Error unmarshaling %s data: %v", event.Type, err)
			return
		}
		h.broadcastToAllUsers(event)
		log.Printf("Broadcasted %s event for reaction %s on target %s (type: %s)", event.Type, reaction.ID.Hex(), reaction.TargetID.Hex(), reaction.TargetType)

	default:
		log.Printf("Received unknown WebSocket event type: %s, data: %s", event.Type, string(event.Data))
	}
}

func (h *Hub) handleNotification(notification models.Notification) {
	notificationJSON, err := json.Marshal(notification)
	if err != nil {
		log.Printf("Error marshaling notification for WebSocket: %v", err)
		return
	}
	wsEvent := models.WebSocketEvent{
		Type: "NOTIFICATION_CREATED",
		Data: notificationJSON,
	}
	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for notification: %v", err)
		return
	}
	h.sendToUser(notification.RecipientID.Hex(), wsEventJSON)
	log.Printf("Sent NOTIFICATION_CREATED event to user %s for notification %s", notification.RecipientID.Hex(), notification.ID.Hex())
}

func (h *Hub) handleReactionEvent(event models.ReactionEvent) {
	reactionEventJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling ReactionEvent for WebSocket: %v", err)
		return
	}
	wsEvent := models.WebSocketEvent{
		Type: "MESSAGE_REACTION_UPDATE",
		Data: reactionEventJSON,
	}
	h.broadcastToParticipants(event.MessageID, wsEvent)
}

func (h *Hub) handleReadReceiptEvent(event models.ReadReceiptEvent) {
	readReceiptEventJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling ReadReceiptEvent for WebSocket: %v", err)
		return
	}
	wsEvent := models.WebSocketEvent{
		Type: "MESSAGE_READ_UPDATE",
		Data: readReceiptEventJSON,
	}
	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for read receipt: %v", err)
		return
	}

	h.sendToUser(event.ReaderID.Hex(), wsEventJSON)

	for _, msgID := range event.MessageIDs {
		msg, err := h.messageRepo.GetMessageByID(context.Background(), msgID)
		if err != nil {
			log.Printf("Error getting message %s for read receipt: %v", msgID.Hex(), err)
			continue
		}
		if msg.SenderID != event.ReaderID {
			h.sendToUser(msg.SenderID.Hex(), wsEventJSON)
		}
	}
}

func (h *Hub) handleMessageEditedEvent(ev models.MessageEditedEvent) {
	data := map[string]string{
		"message_id":  ev.MessageID.Hex(),
		"new_content": ev.NewContent,
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling MESSAGE_EDITED_UPDATE data: %v", err)
		return
	}
	h.broadcastToParticipants(ev.MessageID, models.WebSocketEvent{
		Type: "MESSAGE_EDITED_UPDATE",
		Data: json.RawMessage(dataBytes),
	})
}

func (h *Hub) handleConversationSeenEvent(event models.ConversationSeenEvent) {
	conversationSeenEventJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling ConversationSeenEvent for WebSocket: %v", err)
		return
	}
	wsEvent := models.WebSocketEvent{
		Type: "CONVERSATION_SEEN_UPDATE",
		Data: conversationSeenEventJSON,
	}
	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for conversation seen: %v", err)
		return
	}

	if event.IsGroup {
		group, err := h.groupRepo.GetGroup(context.Background(), event.ConversationID)
		if err != nil {
			log.Printf("Error getting group %s for conversation seen event: %v", event.ConversationID.Hex(), err)
			return
		}
		for _, memberID := range group.Members {
			h.sendToUser(memberID.Hex(), wsEventJSON)
		}
	} else {
		h.sendToUser(event.UserID.Hex(), wsEventJSON)
		h.sendToUser(event.ConversationID.Hex(), wsEventJSON)
	}
}

func (h *Hub) handleDeliveredEvent(dev models.DeliveredEvent) {
	if len(dev.MessageIDs) == 0 {
		return
	}
	msg, err := h.messageRepo.GetMessageByID(h.ctx, dev.MessageIDs[0])
	if err != nil {
		log.Printf("Error getting message %s for delivered processing: %v", dev.MessageIDs[0].Hex(), err)
		return
	}

	var conversationID string
	if !msg.GroupID.IsZero() {
		conversationID = fmt.Sprintf("group_%s", msg.GroupID.Hex())
	} else {
		conversationID = utils.GetConversationID(msg.SenderID, msg.ReceiverID)
	}

	var msgIDs []string
	for _, mid := range dev.MessageIDs {
		msgIDs = append(msgIDs, mid.Hex())
	}

	go func(delivererID primitive.ObjectID, convID string, mIDs []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := h.messageUpdater.MarkMessagesAsDelivered(ctx, delivererID, convID, mIDs)
		if err != nil {
			log.Printf("Error marking messages as delivered: %v", err)
		}
	}(dev.DelivererID, conversationID, msgIDs)

	deliveredEventJSON, err := json.Marshal(dev)
	if err != nil {
		log.Printf("Error marshaling DeliveredEvent for WebSocket: %v", err)
		return
	}
	wsEvent := models.WebSocketEvent{
		Type: "MESSAGE_DELIVERED_UPDATE",
		Data: deliveredEventJSON,
	}

	var clients []*Client
	if !msg.GroupID.IsZero() {
		clients = h.getClientsByGroup(msg.GroupID.Hex())
	} else if !msg.ReceiverID.IsZero() {
		clients = append(h.getClientsByUser(msg.SenderID.Hex()), h.getClientsByUser(msg.ReceiverID.Hex())...)
	}

	wsEventJSON, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for delivered update: %v", err)
		return
	}

	for _, c := range clients {
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

func (h *Hub) handleCallSignal(signal models.CallSignalEvent) {
	signalBytes, err := json.Marshal(signal)
	if err != nil {
		log.Printf("Error marshaling CallSignalEvent: %v", err)
		return
	}

	wsEvent := models.WebSocketEvent{
		Type: "VOICE_CALL_SIGNAL",
		Data: signalBytes,
	}
	wsEventBytes, err := json.Marshal(wsEvent)
	if err != nil {
		log.Printf("Error marshaling WebSocketEvent for call signal: %v", err)
		return
	}

	h.sendToUser(signal.TargetID, wsEventBytes)
	log.Printf("Forwarded VOICE_CALL_SIGNAL (%s) from %s to %s", signal.SignalType, signal.CallerID, signal.TargetID)
}

func (h *Hub) handleEventRSVPEvent(event models.EventRSVPEvent) {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling EventRSVPEvent: %v", err)
		return
	}

	wsEvent := models.WebSocketEvent{
		Type: "EVENT_RSVP_UPDATE",
		Data: eventBytes,
	}
	// broadcast to all users
	h.broadcastToAllUsers(wsEvent)
}
