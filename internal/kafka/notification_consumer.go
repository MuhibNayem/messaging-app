package kafka

import (
	"context"
	"encoding/json"
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/websocket"

	"github.com/segmentio/kafka-go"
)

// NotificationConsumer consumes notification events from Kafka and pushes them to WebSocket clients.
type NotificationConsumer struct {
	reader *kafka.Reader
	hub    *websocket.Hub
}

// NewNotificationConsumer creates a new NotificationConsumer.
func NewNotificationConsumer(brokers []string, topic string, groupID string, hub *websocket.Hub) *NotificationConsumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})
	return &NotificationConsumer{
		reader: r,
		hub:    hub,
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

			var notification models.Notification
			if err := json.Unmarshal(m.Value, &notification); err != nil {
				log.Printf("ERROR: Malformed notification received from Kafka. Topic: %s, Partition: %d, Offset: %d, Error: %v. Message value: %s. Consider sending to a dead-letter queue.", m.Topic, m.Partition, m.Offset, err, string(m.Value))
				c.reader.CommitMessages(ctx, m) // Commit even on unmarshal error to avoid reprocessing bad messages
				continue
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
