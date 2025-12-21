package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"messaging-app/config"
	cassdb "messaging-app/internal/db"
	redisclient "gitlab.com/spydotech-group/shared-entity/redis"
	"messaging-app/internal/repositories"
	"time"

	"github.com/gocql/gocql"
)

// ArchivedMessage represents immutable message content stored in cold storage
type ArchivedMessage struct {
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

// MessageMetadata represents mutable fields that stay in Cassandra
type MessageMetadata struct {
	ConversationID string
	MessageID      gocql.UUID
	Reactions      string
	SeenBy         []string
	DeliveredTo    []string
	IsDeleted      bool
	IsEdited       bool
}

// MessageArchiveService handles tiered storage for messages
type MessageArchiveService struct {
	cassandra     *cassdb.CassandraClient
	storage       *StorageService
	redis         *redisclient.ClusterClient
	archiveBucket string
	cacheTTL      time.Duration
	archiveAfter  int // days
}

// NewMessageArchiveService creates a new archive service
func NewMessageArchiveService(
	cassandra *cassdb.CassandraClient,
	storage *StorageService,
	redisClient *redisclient.ClusterClient,
	cfg *config.Config,
) *MessageArchiveService {
	return &MessageArchiveService{
		cassandra:     cassandra,
		storage:       storage,
		redis:         redisClient,
		archiveBucket: cfg.ArchiveBucket,
		cacheTTL:      time.Duration(cfg.ArchiveCacheTTLMins) * time.Minute,
		archiveAfter:  cfg.ArchiveAfterDays,
	}
}

// ArchiveOldMessages moves old messages to cold storage
// Run as a daily background job
func (s *MessageArchiveService) ArchiveOldMessages(ctx context.Context) error {
	cutoffTime := time.Now().AddDate(0, 0, -s.archiveAfter)
	log.Printf("[Archive] Starting archive job for messages older than %v", cutoffTime)

	// 1. Query old messages from hot table
	query := `SELECT conversation_id, message_id, sender_id, receiver_id, group_id, 
		content, content_type, media_urls, product_id, created_at 
		FROM messages WHERE created_at < ? ALLOW FILTERING`

	iter := s.cassandra.Session.Query(query, cutoffTime).Iter()

	// Group messages by conversation_id and month
	archives := make(map[string]map[string][]ArchivedMessage) // conversation_id -> month -> messages
	var metadataToInsert []struct {
		ConversationID string
		MessageID      gocql.UUID
	}

	var convID, senderID, receiverID, groupID, content, contentType, productID string
	var msgUUID gocql.UUID
	var mediaURLs []string
	var createdAt time.Time

	for iter.Scan(&convID, &msgUUID, &senderID, &receiverID, &groupID, &content, &contentType, &mediaURLs, &productID, &createdAt) {
		month := createdAt.Format("2006-01")

		if archives[convID] == nil {
			archives[convID] = make(map[string][]ArchivedMessage)
		}

		archives[convID][month] = append(archives[convID][month], ArchivedMessage{
			MessageID:   msgUUID.String(),
			SenderID:    senderID,
			ReceiverID:  receiverID,
			GroupID:     groupID,
			Content:     content,
			ContentType: contentType,
			MediaURLs:   mediaURLs,
			ProductID:   productID,
			CreatedAt:   createdAt.Format(time.RFC3339),
		})

		metadataToInsert = append(metadataToInsert, struct {
			ConversationID string
			MessageID      gocql.UUID
		}{convID, msgUUID})
	}

	if err := iter.Close(); err != nil {
		return fmt.Errorf("failed to query old messages: %w", err)
	}

	log.Printf("[Archive] Found %d conversations to archive", len(archives))

	// 2. Upload archives to MinIO and insert index
	for convID, months := range archives {
		for month, messages := range months {
			archivePath := fmt.Sprintf("archives/%s/%s.json.gz", convID, month)

			data, err := json.Marshal(messages)
			if err != nil {
				log.Printf("[Archive] Failed to marshal messages for %s/%s: %v", convID, month, err)
				continue
			}

			if err := s.storage.UploadArchive(ctx, s.archiveBucket, archivePath, data); err != nil {
				log.Printf("[Archive] Failed to upload archive %s: %v", archivePath, err)
				continue
			}

			// Insert archive index
			indexQuery := `INSERT INTO messages_archive_index 
				(conversation_id, month, archive_path, message_count, archived_at) 
				VALUES (?, ?, ?, ?, ?)`
			if err := s.cassandra.Session.Query(indexQuery, convID, month, archivePath, len(messages), time.Now()).Exec(); err != nil {
				log.Printf("[Archive] Failed to insert archive index: %v", err)
			}

			log.Printf("[Archive] Archived %d messages for %s/%s", len(messages), convID, month)
		}
	}

	// 3. Copy metadata to message_metadata table (for reactions, seen_by, etc.)
	for _, m := range metadataToInsert {
		// Copy current metadata (reactions, seen_by, etc.) to metadata table
		copyQuery := `INSERT INTO message_metadata (conversation_id, message_id, reactions, seen_by, delivered_to, is_deleted, is_edited)
			SELECT conversation_id, message_id, reactions, seen_by, delivered_to, is_deleted, false
			FROM messages WHERE conversation_id = ? AND message_id = ?`
		_ = s.cassandra.Session.Query(copyQuery, m.ConversationID, m.MessageID).Exec()
	}

	log.Printf("[Archive] Archive job completed")
	return nil
}

// LoadArchivedMessages loads messages from cold storage with caching
func (s *MessageArchiveService) LoadArchivedMessages(ctx context.Context, conversationID, month string) ([]ArchivedMessage, error) {
	cacheKey := fmt.Sprintf("archive:%s:%s", conversationID, month)

	// 1. Check Redis cache first
	cached, err := s.redis.Get(ctx, cacheKey)
	if err == nil && cached != "" {
		var messages []ArchivedMessage
		if err := json.Unmarshal([]byte(cached), &messages); err == nil {
			return messages, nil
		}
	}

	// 2. Get archive path from index
	var archivePath string
	indexQuery := `SELECT archive_path FROM messages_archive_index WHERE conversation_id = ? AND month = ?`
	if err := s.cassandra.Session.Query(indexQuery, conversationID, month).Scan(&archivePath); err != nil {
		return nil, fmt.Errorf("archive not found for %s/%s: %w", conversationID, month, err)
	}

	// 3. Download from MinIO
	data, err := s.storage.DownloadArchive(ctx, s.archiveBucket, archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to download archive: %w", err)
	}

	var messages []ArchivedMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("failed to parse archive: %w", err)
	}

	// 4. Cache in Redis
	s.redis.Set(ctx, cacheKey, data, s.cacheTTL)

	return messages, nil
}

// GetMessageMetadata gets mutable metadata for archived messages
func (s *MessageArchiveService) GetMessageMetadata(ctx context.Context, conversationID string, messageIDs []gocql.UUID) (map[string]MessageMetadata, error) {
	result := make(map[string]MessageMetadata)

	for _, msgID := range messageIDs {
		var metadata MessageMetadata
		query := `SELECT reactions, seen_by, delivered_to, is_deleted, is_edited 
			FROM message_metadata WHERE conversation_id = ? AND message_id = ?`

		var seenBy, deliveredTo []string
		err := s.cassandra.Session.Query(query, conversationID, msgID).Scan(
			&metadata.Reactions, &seenBy, &deliveredTo, &metadata.IsDeleted, &metadata.IsEdited,
		)
		if err == nil {
			metadata.ConversationID = conversationID
			metadata.MessageID = msgID
			metadata.SeenBy = seenBy
			metadata.DeliveredTo = deliveredTo
			result[msgID.String()] = metadata
		}
	}

	return result, nil
}

// InvalidateCache invalidates Redis cache for a conversation
// Call this when metadata changes (reaction, seen, delete)
func (s *MessageArchiveService) InvalidateCache(ctx context.Context, conversationID string) error {
	// Find all cache keys for this conversation
	pattern := fmt.Sprintf("archive:%s:*", conversationID)
	keys, err := s.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return s.redis.Del(ctx, keys...)
	}
	return nil
}

// StartArchiveWorker starts the background archive job
func (s *MessageArchiveService) StartArchiveWorker(ctx context.Context) {
	// Run daily at 3 AM
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run once immediately on startup (optional, for testing)
	// go s.ArchiveOldMessages(ctx)

	for {
		select {
		case <-ticker.C:
			go s.ArchiveOldMessages(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// ArchiveFetcher interface implementation
// These methods adapt repository interface types to service types

// LoadArchivedMessagesForRepo implements repositories.ArchiveFetcher interface
func (s *MessageArchiveService) LoadArchivedMessagesForRepo(ctx context.Context, conversationID, month string) ([]repositories.ArchivedMessageContent, error) {
	msgs, err := s.LoadArchivedMessages(ctx, conversationID, month)
	if err != nil {
		return nil, err
	}

	result := make([]repositories.ArchivedMessageContent, len(msgs))
	for i, m := range msgs {
		result[i] = repositories.ArchivedMessageContent{
			MessageID:   m.MessageID,
			SenderID:    m.SenderID,
			ReceiverID:  m.ReceiverID,
			GroupID:     m.GroupID,
			Content:     m.Content,
			ContentType: m.ContentType,
			MediaURLs:   m.MediaURLs,
			ProductID:   m.ProductID,
			CreatedAt:   m.CreatedAt,
		}
	}
	return result, nil
}

// GetMessageMetadataForRepo implements repositories.ArchiveFetcher interface
func (s *MessageArchiveService) GetMessageMetadataForRepo(ctx context.Context, conversationID string, messageIDs []string) (map[string]repositories.ArchivedMessageMetadata, error) {
	// Convert string IDs to gocql UUIDs
	var uuids []gocql.UUID
	for _, id := range messageIDs {
		uuid, err := gocql.ParseUUID(id)
		if err == nil {
			uuids = append(uuids, uuid)
		}
	}

	metaMap, err := s.GetMessageMetadata(ctx, conversationID, uuids)
	if err != nil {
		return nil, err
	}

	result := make(map[string]repositories.ArchivedMessageMetadata)
	for k, v := range metaMap {
		result[k] = repositories.ArchivedMessageMetadata{
			Reactions:   v.Reactions,
			SeenBy:      v.SeenBy,
			DeliveredTo: v.DeliveredTo,
			IsDeleted:   v.IsDeleted,
			IsEdited:    v.IsEdited,
		}
	}
	return result, nil
}
