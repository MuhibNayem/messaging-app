package kafka

import (
	"context"
	"encoding/json"
	"log"
	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/repositories"
	"messaging-app/internal/websocket"
	"gitlab.com/spydotech-group/shared-entity/events"
	"gitlab.com/spydotech-group/shared-entity/kafka"
	"time"

	segmentio "github.com/segmentio/kafka-go"
)

// NotificationConsumer consumes notification events from Kafka and pushes them to WebSocket clients.
type NotificationConsumer struct {
	reader      *segmentio.Reader
	hub         *websocket.Hub
	repo        *repositories.NotificationRepository
	dlqProducer *kafka.DLQProducer
}

// NewNotificationConsumer creates a new NotificationConsumer.
func NewNotificationConsumer(brokers []string, topic string, groupID string, hub *websocket.Hub, repo *repositories.NotificationRepository, dlq *kafka.DLQProducer) *NotificationConsumer {
	r := segmentio.NewReader(segmentio.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})
	return &NotificationConsumer{
		reader:      r,
		hub:         hub,
		repo:        repo,
		dlqProducer: dlq,
	}
}

// Start consuming messages from Kafka.
func (c *NotificationConsumer) Start(ctx context.Context) {
	log.Printf("Starting Kafka Notification Consumer for topic %s", c.reader.Config().Topic)
	for {
		select {
		case <-ctx.Done():
			log.Printf("Kafka Notification Consumer for topic %s stopped", c.reader.Config().Topic)
			return
		default:
			m, err := c.reader.FetchMessage(ctx)
			if err != nil {
				log.Printf("Error fetching message from Kafka: %v", err)
				// Depending on the error, you might want to commit or not. For now, continue.
				continue
			}

			var event events.NotificationCreatedEvent
			if err := json.Unmarshal(m.Value, &event); err != nil {
				log.Printf("ERROR: Malformed notification received from Kafka. Topic: %s, Partition: %d, Offset: %d, Error: %v. Message value: %s. Sending to DLQ.", m.Topic, m.Partition, m.Offset, err, string(m.Value))
				// Send to DLQ since we can't parse it
				if dlqErr := c.dlqProducer.PublishDeadLetter(ctx, c.reader.Config().Topic, m.Value, err); dlqErr != nil {
					log.Printf("FATAL: Failed to send malformed message to DLQ: %v", dlqErr)
				}
				c.reader.CommitMessages(ctx, m) // Commit to avoid processing bad message loop
				continue
			}

			notification := models.Notification{
				ID:          event.ID,
				RecipientID: event.RecipientID,
				SenderID:    event.SenderID,
				Type:        models.NotificationType(event.Type),
				TargetID:    event.TargetID,
				TargetType:  event.TargetType,
				Content:     event.Content,
				Data:        event.Data,
				Read:        event.Read,
				CreatedAt:   event.CreatedAt,
			}

			// Persist to DB with robust retry mechanism
			// We retry DB writes to ensure data consistency.
			maxRetries := 3
			for i := 0; i < maxRetries; i++ {
				dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, saveErr := c.repo.CreateNotification(dbCtx, &notification)
				cancel()
				if saveErr == nil {
					break
				}

				if i < maxRetries-1 {
					log.Printf("Error persisting notification to DB (attempt %d/%d): %v. Retrying...", i+1, maxRetries, saveErr)
					time.Sleep(time.Second * time.Duration(i+1)) // Exponential backoff
				} else {
					log.Printf("CRITICAL: Failed to persist notification after %d attempts. Sending to DLQ. Error: %v. Message ID: %s", maxRetries, saveErr, notification.ID.Hex())
					if err := c.dlqProducer.PublishDeadLetter(ctx, c.reader.Config().Topic, m.Value, saveErr); err != nil {
						log.Printf("FATAL: Failed to send to DLQ: %v", err)
					}
					// We proceed to commit the message to avoid head-of-line blocking.
				}
			}

			// Push notification to the WebSocket hub in a non-blocking way
			select {
			case c.hub.NotificationEvents <- notification:
				// Successfully sent to hub
			default:
				log.Printf("WARNING: WebSocket hub's NotificationEvents channel is full. Dropping real-time notification for recipient %s. Notification will still be available in DB.", notification.RecipientID.Hex())
				// In a high-volume scenario, you might want to implement a separate retry queue for WebSocket delivery
			}

			if err := c.reader.CommitMessages(ctx, m); err != nil {
				log.Printf("Error committing message to Kafka: %v", err)
			}
		}
	}
}

// Close closes the Kafka reader.
func (c *NotificationConsumer) Close() error {
	return c.reader.Close()
}
