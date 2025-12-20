package kafka

import (
	"context"
	"encoding/json"
	"log"
	"messaging-app/internal/models"
	"messaging-app/internal/websocket"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"
)

var (
	messagesConsumed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_messages_consumed_total",
			Help: "Total number of messages consumed from Kafka",
		},
		[]string{"topic"},
	)
	consumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kafka_consume_duration_seconds",
			Help:    "Duration of Kafka consume operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic"},
	)
)

type MessageConsumer struct {
	reader *kafka.Reader
	hub    *websocket.Hub
}

func NewMessageConsumer(brokers []string, topic string, groupID string, hub *websocket.Hub) *MessageConsumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
	})

	return &MessageConsumer{
		reader: r,
		hub:    hub,
	}
}

func (c *MessageConsumer) ConsumeMessages(ctx context.Context) {
	for {
		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			log.Printf("Error fetching message: %v", err)
			break
		}

		messagesConsumed.WithLabelValues(m.Topic).Inc()
		start := time.Now()

		// Attempt to unmarshal as a Message
		var msg models.Message
		if err := json.Unmarshal(m.Value, &msg); err == nil && !msg.ID.IsZero() {
			log.Printf("Received Kafka message of type: Message for topic %s at offset %d", m.Topic, m.Offset)
			c.hub.Broadcast <- msg
		} else {
			// If not a Message, check for other types.
			// ReactionEvent and MessageEditedEvent share 'message_id' key, so we need strict checks.

			// Check for ReactionEvent: Must have Emoji and Action
			var reactionEvent models.ReactionEvent
			if err := json.Unmarshal(m.Value, &reactionEvent); err == nil && !reactionEvent.MessageID.IsZero() && reactionEvent.Emoji != "" {
				log.Printf("Received Kafka message of type: ReactionEvent for topic %s at offset %d", m.Topic, m.Offset)
				c.hub.ReactionEvents <- reactionEvent
			} else {
				// If not a ReactionEvent, attempt to unmarshal as a ReadReceiptEvent
				var readReceiptEvent models.ReadReceiptEvent
				if err := json.Unmarshal(m.Value, &readReceiptEvent); err == nil && len(readReceiptEvent.MessageIDs) > 0 {
					log.Printf("Received Kafka message of type: ReadReceiptEvent for topic %s at offset %d", m.Topic, m.Offset)
					c.hub.ReadReceiptEvents <- readReceiptEvent
				} else {
					// If not a ReadReceiptEvent, attempt to unmarshal as a MessageEditedEvent
					var messageEditedEvent models.MessageEditedEvent
					// Must have NewContent or EditorID. Note: Content could be empty string potentially?
					// But usually not. Let's check EditorID too.
					if err := json.Unmarshal(m.Value, &messageEditedEvent); err == nil && !messageEditedEvent.MessageID.IsZero() && !messageEditedEvent.EditorID.IsZero() {
						log.Printf("Received Kafka message of type: MessageEditedEvent for topic %s at offset %d", m.Topic, m.Offset)
						c.hub.MessageEditedEvents <- messageEditedEvent
					} else {
						// If not a MessageEditedEvent, attempt to unmarshal as a ConversationSeenEvent
						var conversationSeenEvent models.ConversationSeenEvent
						if err := json.Unmarshal(m.Value, &conversationSeenEvent); err == nil && !conversationSeenEvent.ConversationID.IsZero() {

							log.Printf("Received Kafka message of type: ConversationSeenEvent for topic %s at offset %d", m.Topic, m.Offset)
							c.hub.ConversationSeenEvents <- conversationSeenEvent
						} else {
							// Fallback to WebSocketEvent (for feed events)
							var wsEvent models.WebSocketEvent
							if err := json.Unmarshal(m.Value, &wsEvent); err != nil {
								log.Printf("Error unmarshaling Kafka message to known types or WebSocketEvent: %v, message: %s", err, string(m.Value))
								// If unmarshaling fails, commit the message to avoid reprocessing
								if err := c.reader.CommitMessages(ctx, m); err != nil {
									log.Printf("Error committing message after unmarshaling failure: %v", err)
								}
								continue
							}
							log.Printf("Received Kafka event of type: %s for topic %s at offset %d", wsEvent.Type, m.Topic, m.Offset)
							c.hub.FeedEvents <- wsEvent
						}
					}
				}
			}
		}

		if err := c.reader.CommitMessages(ctx, m); err != nil {
			log.Printf("Error committing message: %v", err)
		}

		consumeDuration.WithLabelValues(m.Topic).Observe(time.Since(start).Seconds())
	}

	if err := c.reader.Close(); err != nil {
		log.Printf("Error closing Kafka reader: %v", err)
	}
}

func (c *MessageConsumer) Close() error {
	return c.reader.Close()
}
