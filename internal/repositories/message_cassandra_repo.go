package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"messaging-app/internal/db"
	"gitlab.com/spydotech-group/shared-entity/models"
	"sort"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ArchiveFetcher interface for loading archived messages (implemented by MessageArchiveService)
// This avoids circular dependency between repository and service
type ArchiveFetcher interface {
	LoadArchivedMessagesForRepo(ctx context.Context, conversationID, month string) ([]ArchivedMessageContent, error)
	GetMessageMetadataForRepo(ctx context.Context, conversationID string, messageIDs []string) (map[string]ArchivedMessageMetadata, error)
}

// ArchivedMessageContent represents immutable content from cold storage
type ArchivedMessageContent struct {
	MessageID   string   `json:"message_id"`
	SenderID    string   `json:"sender_id"`
	ReceiverID  string   `json:"receiver_id,omitempty"`
	GroupID     string   `json:"group_id,omitempty"`
	Content     string   `json:"content"`
	ContentType string   `json:"content_type"`
	MediaURLs   []string `json:"media_urls,omitempty"`
	ProductID   string   `json:"product_id,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

// ArchivedMessageMetadata represents mutable metadata from hot storage
type ArchivedMessageMetadata struct {
	Reactions   string
	SeenBy      []string
	DeliveredTo []string
	IsDeleted   bool
	IsEdited    bool
}

type MessageCassandraRepository struct {
	client         *db.CassandraClient
	archiveFetcher ArchiveFetcher // Optional, for loading archived messages
}

func NewMessageCassandraRepository(client *db.CassandraClient) *MessageCassandraRepository {
	return &MessageCassandraRepository{client: client}
}

// SetArchiveFetcher sets the archive fetcher for loading cold storage messages
func (r *MessageCassandraRepository) SetArchiveFetcher(fetcher ArchiveFetcher) {
	r.archiveFetcher = fetcher
}

// getConversationID derives a deterministic conversation ID for DMs or Groups.
// For DMs, it sorts user IDs to ensure A->B and B->A map to the same conversation.
func getConversationID(senderId, receiverId, groupId primitive.ObjectID) string {
	if !groupId.IsZero() {
		return "group_" + groupId.Hex()
	}
	if senderId.Hex() < receiverId.Hex() {
		return "dm_" + senderId.Hex() + "_" + receiverId.Hex()
	}
	return "dm_" + receiverId.Hex() + "_" + senderId.Hex()
}

// InboxParams holds metadata for populating the inbox view (names/avatars)
type InboxParams struct {
	SenderName     string
	SenderAvatar   string
	ReceiverName   string
	ReceiverAvatar string
	GroupName      string
	GroupAvatar    string
	IsGroup        bool
}

// Create persists a new message to Cassandra and updates inboxes with rich metadata.
func (r *MessageCassandraRepository) Create(ctx context.Context, msg *models.Message, recipientIDs []primitive.ObjectID, params InboxParams) error {
	if r.client == nil || r.client.Session == nil {
		return fmt.Errorf("cassandra client not initialized")
	}

	// --- VALIDATION: Prevent corrupt data at write time (Facebook-scale defense) ---
	// This ensures data integrity at the source, preventing ghost messages
	if msg.SenderID.IsZero() {
		return fmt.Errorf("message validation failed: sender_id cannot be empty")
	}
	if msg.ContentType == "" {
		return fmt.Errorf("message validation failed: content_type cannot be empty")
	}
	if msg.Content == "" && len(msg.MediaURLs) == 0 {
		return fmt.Errorf("message validation failed: message must have content or media")
	}
	// --- END VALIDATION ---

	// 1. Prepare Data
	conversationID := getConversationID(msg.SenderID, msg.ReceiverID, msg.GroupID)

	// Parse or generate the Cassandra TimeUUID for this message
	var messageUUID gocql.UUID
	if msg.StringID != "" {
		if parsed, err := gocql.ParseUUID(msg.StringID); err == nil {
			messageUUID = parsed
		}
	}
	if messageUUID == (gocql.UUID{}) {
		messageUUID = gocql.TimeUUID()
		msg.StringID = messageUUID.String()
	}

	if msg.ID.IsZero() {
		msg.ID = primitive.NewObjectID()
	}

	reactionsJSON, err := json.Marshal(msg.Reactions)
	if err != nil {
		log.Printf("Error marshaling reactions: %v", err)
		reactionsJSON = []byte("[]")
	}

	// 2. Prepare Batch
	batch := r.client.Session.NewBatch(gocql.LoggedBatch)

	// Statement A: Insert into messages
	const insertMessageQuery = `INSERT INTO messages (
		conversation_id, message_id, sender_id, receiver_id, group_id, 
		content, content_type, media_urls, is_read, 
		is_marketplace, product_id, reactions, created_at, is_deleted
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	batch.Query(insertMessageQuery,
		conversationID, messageUUID, msg.SenderID.Hex(), msg.ReceiverID.Hex(), msg.GroupID.Hex(),
		msg.Content, msg.ContentType, msg.MediaURLs, false,
		msg.IsMarketplace, getStrID(msg.ProductID), string(reactionsJSON), msg.CreatedAt, false,
	)

	// Statement B: Update Inbox (for Sender and all Recipients)
	const insertInboxQuery = `INSERT INTO user_inbox (
		user_id, conversation_id, conversation_name, conversation_avatar, 
		is_group, is_marketplace, last_message_content, last_message_sender_id, last_message_sender_name, last_message_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Helper to decide name/avatar based on whose inbox we are writing to
	getInboxMetadata := func(ownerID string) (string, string) {
		if params.IsGroup {
			return params.GroupName, params.GroupAvatar
		}
		// Direct Message Logic
		if ownerID == msg.SenderID.Hex() {
			return params.ReceiverName, params.ReceiverAvatar // Sender sees Receiver
		}
		return params.SenderName, params.SenderAvatar // Receiver sees Sender
	}

	// 1. Sender's Inbox
	sName, sAvatar := getInboxMetadata(msg.SenderID.Hex())
	batch.Query(insertInboxQuery,
		msg.SenderID.Hex(), conversationID, sName, sAvatar,
		params.IsGroup, msg.IsMarketplace, msg.Content, msg.SenderID.Hex(), msg.SenderName, msg.CreatedAt,
	)

	// 2. Recipients' Inboxes
	for _, rid := range recipientIDs {
		// Avoid duplicate insert for sender if they are in recipient list (shouldn't happen with correct service logic, but safe to check)
		if rid.Hex() == msg.SenderID.Hex() {
			continue
		}
		rName, rAvatar := getInboxMetadata(rid.Hex())
		batch.Query(insertInboxQuery,
			rid.Hex(), conversationID, rName, rAvatar,
			params.IsGroup, msg.IsMarketplace, msg.Content, msg.SenderID.Hex(), msg.SenderName, msg.CreatedAt,
		)
	}

	// 3. Execute Batch
	if err := r.client.Session.ExecuteBatch(batch); err != nil {
		return err
	}

	// 4. Update Unread Counters (Async)
	go func(recipients []primitive.ObjectID, convID string) {
		const updateCounterQuery = `UPDATE conversation_unread SET unread_count = unread_count + 1 WHERE user_id = ? AND conversation_id = ?`
		for _, rid := range recipients {
			if err := r.client.Session.Query(updateCounterQuery, rid.Hex(), convID).Exec(); err != nil {
				log.Printf("Error incrementing unread count for %s: %v", rid.Hex(), err)
			}
		}
	}(recipientIDs, conversationID)

	return nil
}

// GetInbox retrieves the conversation list for a user, segregated by marketplace flag.
func (r *MessageCassandraRepository) GetInbox(ctx context.Context, userID primitive.ObjectID, isMarketplace bool) ([]models.ConversationSummary, error) {
	if r.client == nil || r.client.Session == nil {
		return nil, fmt.Errorf("cassandra client not initialized")
	}

	// Query user_inbox (Partition: user_id) - O(1) partition read
	// We CAN filter by is_marketplace because it is the first Clustering Key
	query := `SELECT conversation_id, conversation_name, conversation_avatar, is_group, last_message_content, last_message_sender_id, last_message_sender_name, last_message_at 
	          FROM user_inbox WHERE user_id = ? AND is_marketplace = ?`

	iter := r.client.Session.Query(query, userID.Hex(), isMarketplace).Iter()

	// Fetch ALL unread counts for this user in a SINGLE query - O(1) partition read
	// This eliminates N+1 query problem for scalability
	unreadQuery := `SELECT conversation_id, unread_count FROM conversation_unread WHERE user_id = ?`
	unreadIter := r.client.Session.Query(unreadQuery, userID.Hex()).Iter()

	unreadMap := make(map[string]int64)
	var ucConvID string
	var ucCount int64
	for unreadIter.Scan(&ucConvID, &ucCount) {
		unreadMap[ucConvID] = ucCount
	}
	if err := unreadIter.Close(); err != nil {
		log.Printf("Error fetching unread counts: %v", err)
	}

	var summaries = []models.ConversationSummary{}
	var convID, name, avatar, lastMsgContent, lastMsgSenderID, lastMsgSenderName string
	var isGroup bool
	var lastMsgAt time.Time

	for iter.Scan(&convID, &name, &avatar, &isGroup, &lastMsgContent, &lastMsgSenderID, &lastMsgSenderName, &lastMsgAt) {
		sid, _ := primitive.ObjectIDFromHex(lastMsgSenderID)

		// Lookup unread count from in-memory map - O(1)
		unread := unreadMap[convID]

		// Transform internal Cassandra conversation ID to frontend format
		frontendID := convID
		if !isGroup && strings.HasPrefix(convID, "dm_") {
			parts := strings.Split(convID, "_")
			if len(parts) == 3 {
				if parts[1] == userID.Hex() {
					frontendID = "user-" + parts[2]
				} else {
					frontendID = "user-" + parts[1]
				}
			}
		} else if isGroup && strings.HasPrefix(convID, "group_") {
			frontendID = "group-" + convID[6:]
		} else if isGroup {
			frontendID = "group-" + convID
		}

		summaries = append(summaries, models.ConversationSummary{
			ID:                     frontendID,
			Name:                   name,
			Avatar:                 avatar,
			IsGroup:                isGroup,
			LastMessageContent:     lastMsgContent,
			LastMessageSenderID:    sid,
			LastMessageSenderName:  lastMsgSenderName,
			LastMessageTimestamp:   &lastMsgAt,
			UnreadCount:            unread,
			LastMessageIsEncrypted: false,
		})
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	// Sort by last_message_at DESC - O(N log N) where N = user's conversations (typically <100)
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].LastMessageTimestamp == nil {
			return false
		}
		if summaries[j].LastMessageTimestamp == nil {
			return true
		}
		return summaries[i].LastMessageTimestamp.After(*summaries[j].LastMessageTimestamp)
	})

	log.Printf("[] Successfully retrieved %d conversation summaries for user %s", len(summaries), userID.Hex())
	return summaries, nil
}

// GetMessages retrieves paginated messages for a conversation.
func (r *MessageCassandraRepository) GetMessages(ctx context.Context, query models.MessageQuery) ([]models.Message, error) {
	if r.client == nil || r.client.Session == nil {
		return nil, fmt.Errorf("cassandra client not initialized")
	}

	// 1. Derive Context
	var conversationID string

	if query.ConversationID != "" {
		conversationID = query.ConversationID
	} else {
		var senderID, receiverID, groupID primitive.ObjectID

		if query.GroupID != "" {
			groupID, _ = primitive.ObjectIDFromHex(query.GroupID)
		}
		if query.SenderID != "" {
			senderID, _ = primitive.ObjectIDFromHex(query.SenderID)
		}
		if query.ReceiverID != "" {
			receiverID, _ = primitive.ObjectIDFromHex(query.ReceiverID)
		}
		conversationID = getConversationID(senderID, receiverID, groupID)
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 50 // Default page size
	}

	// 2. Build Query
	var cqlQuery string
	var iter *gocql.Iter

	// Cassandra optimized pagination uses 'message_id' clustering key (TimeUUID)
	// Updated columns to include receiver_id, group_id, is_marketplace, product_id, seen_by, delivered_to
	columns := "message_id, sender_id, receiver_id, group_id, content, created_at, reactions, media_urls, is_marketplace, content_type, product_id, seen_by, delivered_to"
	if query.Before == "" {
		cqlQuery = fmt.Sprintf(`SELECT %s FROM messages WHERE conversation_id = ? LIMIT ?`, columns)
		iter = r.client.Session.Query(cqlQuery, conversationID, limit).Iter()
	} else {
		beforeTime, err := time.Parse(time.RFC3339, query.Before)
		if err != nil {
			log.Printf("Invalid time format for 'before' param: %v. Fallback to latest.", err)
			cqlQuery = fmt.Sprintf(`SELECT %s FROM messages WHERE conversation_id = ? LIMIT ?`, columns)
			iter = r.client.Session.Query(cqlQuery, conversationID, limit).Iter()
		} else {
			// Create a TimeUUID from the beforeTime to use for pagination
			// We want messages strictly BEFORE this time.
			// UUIDFromTime creates a UUID with the given time.
			// Since we sort DESC, `message_id < ?` gives us older messages.
			maxTimeUUID := gocql.UUIDFromTime(beforeTime)
			cqlQuery = fmt.Sprintf(`SELECT %s FROM messages WHERE conversation_id = ? AND message_id < ? LIMIT ?`, columns)
			iter = r.client.Session.Query(cqlQuery, conversationID, maxTimeUUID, limit).Iter()
		}
	}

	// 3. Scan Results
	var messages []models.Message
	var sID, rID, gID, content, reactions, contentType, productID string
	var msgUUID gocql.UUID
	var createdAt time.Time
	var mediaUrls []string
	var isMarketplace bool
	var seenByStr, deliveredToStr []string

	for iter.Scan(&msgUUID, &sID, &rID, &gID, &content, &createdAt, &reactions, &mediaUrls, &isMarketplace, &contentType, &productID, &seenByStr, &deliveredToStr) {
		sid, _ := primitive.ObjectIDFromHex(sID)

		var rid, gid primitive.ObjectID
		if rID != "" {
			rid, _ = primitive.ObjectIDFromHex(rID)
		}
		if gID != "" {
			gid, _ = primitive.ObjectIDFromHex(gID)
		}

		var parsedReactions []models.MessageReaction
		if reactions != "" {
			_ = json.Unmarshal([]byte(reactions), &parsedReactions)
		}

		var pid *primitive.ObjectID
		if productID != "" {
			parsedPID, err := primitive.ObjectIDFromHex(productID)
			if err == nil {
				pid = &parsedPID
			}
		}

		// Convert seen_by and delivered_to strings to ObjectIDs
		var seenBy, deliveredTo []primitive.ObjectID
		for _, s := range seenByStr {
			if oid, err := primitive.ObjectIDFromHex(s); err == nil {
				seenBy = append(seenBy, oid)
			}
		}
		for _, d := range deliveredToStr {
			if oid, err := primitive.ObjectIDFromHex(d); err == nil {
				deliveredTo = append(deliveredTo, oid)
			}
		}

		// --- DATA INTEGRITY CHECK: Skip corrupt/malformed rows (Facebook-scale defense) ---
		// This filtering happens at the repository layer for efficiency
		// (avoids passing corrupt data through the entire service stack)
		if sid.IsZero() && content == "" && contentType == "" {
			log.Printf("[DATA_INTEGRITY] Skipping malformed message row: %s (empty sender, content, content_type)", msgUUID.String())
			continue
		}

		messages = append(messages, models.Message{
			ID:            primitive.NewObjectID(), // Placeholder
			StringID:      msgUUID.String(),
			SenderID:      sid,
			ReceiverID:    rid,
			GroupID:       gid,
			Content:       content,
			ContentType:   contentType,
			CreatedAt:     createdAt,
			Reactions:     parsedReactions,
			MediaURLs:     mediaUrls,
			IsMarketplace: isMarketplace,
			ProductID:     pid,
			SeenBy:        seenBy,
			DeliveredTo:   deliveredTo,
		})
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	// 4. If fewer messages than requested and archive fetcher available, check cold storage
	if len(messages) < limit && r.archiveFetcher != nil {
		// Determine the month to check for archived messages
		var oldestTime time.Time
		if len(messages) > 0 {
			oldestTime = messages[len(messages)-1].CreatedAt
		} else if query.Before != "" {
			oldestTime, _ = time.Parse(time.RFC3339, query.Before)
		} else {
			oldestTime = time.Now().AddDate(0, 0, -30) // Default 30 days ago
		}

		// Check if we're scrolling into archived territory (older than 30 days)
		archiveThreshold := time.Now().AddDate(0, 0, -30)
		if oldestTime.Before(archiveThreshold) {
			month := oldestTime.Format("2006-01")
			archivedContent, err := r.archiveFetcher.LoadArchivedMessagesForRepo(ctx, conversationID, month)
			if err == nil && len(archivedContent) > 0 {
				// Get metadata for archived messages
				var msgIDs []string
				for _, m := range archivedContent {
					msgIDs = append(msgIDs, m.MessageID)
				}
				metaMap, _ := r.archiveFetcher.GetMessageMetadataForRepo(ctx, conversationID, msgIDs)

				// Convert to models.Message and merge
				for _, archived := range archivedContent {
					sid, _ := primitive.ObjectIDFromHex(archived.SenderID)
					var rid, gid primitive.ObjectID
					if archived.ReceiverID != "" {
						rid, _ = primitive.ObjectIDFromHex(archived.ReceiverID)
					}
					if archived.GroupID != "" {
						gid, _ = primitive.ObjectIDFromHex(archived.GroupID)
					}

					createdAt, _ := time.Parse(time.RFC3339, archived.CreatedAt)

					// Apply metadata if available
					var parsedReactions []models.MessageReaction
					if meta, ok := metaMap[archived.MessageID]; ok {
						if meta.Reactions != "" {
							_ = json.Unmarshal([]byte(meta.Reactions), &parsedReactions)
						}
						if meta.IsDeleted {
							continue // Skip deleted messages
						}
					}

					var pid *primitive.ObjectID
					if archived.ProductID != "" {
						parsedPID, err := primitive.ObjectIDFromHex(archived.ProductID)
						if err == nil {
							pid = &parsedPID
						}
					}

					messages = append(messages, models.Message{
						ID:          primitive.NewObjectID(),
						StringID:    archived.MessageID,
						SenderID:    sid,
						ReceiverID:  rid,
						GroupID:     gid,
						Content:     archived.Content,
						ContentType: archived.ContentType,
						CreatedAt:   createdAt,
						Reactions:   parsedReactions,
						MediaURLs:   archived.MediaURLs,
						ProductID:   pid,
					})
				}

				// Re-sort by time descending and limit
				sort.Slice(messages, func(i, j int) bool {
					return messages[i].CreatedAt.After(messages[j].CreatedAt)
				})
				if len(messages) > limit {
					messages = messages[:limit]
				}
			}
		}
	}

	return messages, nil
}

// GetTotalUnreadCount sums up unread counts from all conversations for a user
func (r *MessageCassandraRepository) GetTotalUnreadCount(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	if r.client == nil || r.client.Session == nil {
		return 0, fmt.Errorf("cassandra client not initialized")
	}

	// In Cassandra, iterating all conversation_unread rows for a user might be slow if they have thousands.
	// Optimize: Use a counter table 'user_unread_total' or just iterate (acceptable for MVP).
	// Query: SELECT unread_count FROM conversation_unread WHERE user_id = ?
	// Note: conversation_unread partition key is (user_id, conversation_id).
	// To query by just user_id, we need a secondary index or a different table design.
	// Current schema check: Is user_id the partition key?
	// If PRIMARY KEY ((user_id), conversation_id), then we CAN query by user_id.
	// Let's assume schema is PRIMARY KEY ((user_id), conversation_id) for now.

	query := `SELECT unread_count FROM conversation_unread WHERE user_id = ?`
	iter := r.client.Session.Query(query, userID.Hex()).Iter()

	var total int64 = 0
	var count int64
	for iter.Scan(&count) {
		total += count
	}

	if err := iter.Close(); err != nil {
		return 0, err
	}
	return total, nil
}

// MarkConversationAsSeen resets unread count for a conversation
// For Cassandra counter columns, we DELETE the row to reset (counters can't use SET = 0)
// The row will be recreated with 0 on next increment
func (r *MessageCassandraRepository) MarkConversationAsSeen(ctx context.Context, userID primitive.ObjectID, conversationID string) error {
	if r.client == nil || r.client.Session == nil {
		return fmt.Errorf("cassandra client not initialized")
	}

	query := `DELETE FROM conversation_unread WHERE user_id = ? AND conversation_id = ?`
	return r.client.Session.Query(query, userID.Hex(), conversationID).Exec()
}

// MarkMessagesAsSeen updates the is_read flag and adds user to seen_by for specific messages
func (r *MessageCassandraRepository) MarkMessagesAsSeen(ctx context.Context, conversationID string, messageIDs []string, userID string) error {
	if r.client == nil || r.client.Session == nil {
		return fmt.Errorf("cassandra client not initialized")
	}

	// Cassandra Batch Update
	batch := r.client.Session.NewBatch(gocql.LoggedBatch)
	// Update is_read and add user to seen_by SET
	query := `UPDATE messages SET is_read = true, seen_by = seen_by + ? WHERE conversation_id = ? AND message_id = ?`

	for _, msgID := range messageIDs {
		// We need UUIDs, assuming messageIDs are TimeUUID strings
		uuid, err := gocql.ParseUUID(msgID)
		if err != nil {
			log.Printf("Invalid UUID for message seen update: %s", msgID)
			continue
		}
		// Add user to seen_by set
		batch.Query(query, []string{userID}, conversationID, uuid)
	}

	return r.client.Session.ExecuteBatch(batch)
}

// MarkMessagesAsDelivered updates the delivered_to list for specific messages
// Optimized for scale: uses concurrent queries to fetch created_at timestamps
func (r *MessageCassandraRepository) MarkMessagesAsDelivered(ctx context.Context, conversationID string, messageIDs []string, userID string) error {
	if r.client == nil || r.client.Session == nil {
		return fmt.Errorf("cassandra client not initialized")
	}

	if len(messageIDs) == 0 {
		return nil
	}

	// Security: Validate userID is a valid MongoDB ObjectID hex string (24 hex chars)
	if len(userID) != 24 {
		return fmt.Errorf("invalid userID length: expected 24, got %d", len(userID))
	}
	for _, c := range userID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("invalid userID: contains non-hex character")
		}
	}

	// Cassandra PRIMARY KEY is ((conversation_id), message_id)
	// We no longer need created_at for UPDATEs. Just conversation_id and message_id.

	// Parse UUIDs
	var validMsgIDs []gocql.UUID
	for _, msgID := range messageIDs {
		uuid, err := gocql.ParseUUID(msgID)
		if err != nil {
			log.Printf("Invalid UUID for message delivered update: %s", msgID)
			continue
		}
		validMsgIDs = append(validMsgIDs, uuid)
	}

	if len(validMsgIDs) == 0 {
		return nil
	}

	// Batch Update directly without lookup
	batch := r.client.Session.NewBatch(gocql.LoggedBatch)
	updateQuery := fmt.Sprintf(`UPDATE messages SET delivered_to = delivered_to + {'%s'} WHERE conversation_id = ? AND message_id = ?`, userID)

	for _, uuid := range validMsgIDs {
		batch.Query(updateQuery, conversationID, uuid)
	}

	return r.client.Session.ExecuteBatch(batch)
}

func getStrID(id *primitive.ObjectID) string {
	if id == nil {
		return ""
	}
	return id.Hex()
}

// DeleteMessage performs a soft delete on a message
func (r *MessageCassandraRepository) DeleteMessage(ctx context.Context, conversationID string, messageID string) error {
	if r.client == nil || r.client.Session == nil {
		return fmt.Errorf("cassandra client not initialized")
	}

	uuid, err := gocql.ParseUUID(messageID)
	if err != nil {
		return fmt.Errorf("invalid message UUID: %w", err)
	}

	query := `UPDATE messages SET is_deleted = true, content = '[Message Deleted]', media_urls = [] WHERE conversation_id = ? AND message_id = ?`
	return r.client.Session.Query(query, conversationID, uuid).Exec()
}

// EditMessage updates the content of a message (if not deleted)
func (r *MessageCassandraRepository) EditMessage(ctx context.Context, conversationID string, messageID string, newContent string) error {
	if r.client == nil || r.client.Session == nil {
		return fmt.Errorf("cassandra client not initialized")
	}

	uuid, err := gocql.ParseUUID(messageID)
	if err != nil {
		return fmt.Errorf("invalid message UUID: %w", err)
	}

	query := `UPDATE messages SET content = ?, is_edited = true WHERE conversation_id = ? AND message_id = ?`
	return r.client.Session.Query(query, newContent, conversationID, uuid).Exec()
}

// SearchMessages performs a basic search (Limited Support in Cassandra)
func (r *MessageCassandraRepository) SearchMessages(ctx context.Context, userID primitive.ObjectID, query string, page, limit int64) ([]models.Message, error) {
	// Full text search requires SASI or external index.
	// For migration compliance, we return empty list or log warning.
	// Implementing exact match on content using ALLOW FILTERING (Inefficient - Dev only) or just stub.
	log.Println("WARNING: SearchMessages is not fully supported in Cassandra mode. Returning empty results.")
	return []models.Message{}, nil
}

// GetMarketplacePartnerIDs returns unique user IDs from marketplace conversations for presence broadcasting
func (r *MessageCassandraRepository) GetMarketplacePartnerIDs(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	if r.client == nil || r.client.Session == nil {
		return nil, fmt.Errorf("cassandra client not initialized")
	}

	// Query user_inbox for marketplace conversations
	// conversation_id format is "dm_userA_userB" for DMs
	query := `SELECT conversation_id FROM user_inbox WHERE user_id = ? AND is_marketplace = true`
	iter := r.client.Session.Query(query, userID.Hex()).Iter()

	partnerMap := make(map[string]bool)
	var conversationID string
	userIDHex := userID.Hex()

	for iter.Scan(&conversationID) {
		// Parse conversation_id to extract partner ID
		// Format: "dm_userA_userB" where userA < userB alphabetically
		if strings.HasPrefix(conversationID, "dm_") {
			parts := strings.Split(conversationID[3:], "_")
			if len(parts) == 2 {
				if parts[0] == userIDHex {
					partnerMap[parts[1]] = true
				} else if parts[1] == userIDHex {
					partnerMap[parts[0]] = true
				}
			}
		}
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	// Convert to ObjectIDs
	var partnerIDs []primitive.ObjectID
	for partnerHex := range partnerMap {
		if oid, err := primitive.ObjectIDFromHex(partnerHex); err == nil {
			partnerIDs = append(partnerIDs, oid)
		}
	}

	return partnerIDs, nil
}
