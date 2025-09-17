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

		var wsEvent models.WebSocketEvent
		if err := json.Unmarshal(m.Value, &wsEvent); err != nil {
			log.Printf("Error unmarshaling Kafka message to WebSocketEvent: %v, message: %s", err, string(m.Value))
			// If unmarshaling fails, commit the message to avoid reprocessing
			if err := c.reader.CommitMessages(ctx, m); err != nil {
				log.Printf("Error committing message after unmarshaling failure: %v", err)
			}
			continue
		}

		log.Printf("Received Kafka event of type: %s for topic %s at offset %d", wsEvent.Type, m.Topic, m.Offset)

		// Send the WebSocketEvent to the Hub's FeedEvents channel
		c.hub.FeedEvents <- wsEvent

		if err := c.reader.CommitMessages(ctx, m); err != nil {
			log.Printf("Error committing message: %v", err)
		}

		consumeDuration.WithLabelValues(m.Topic).Observe(time.Since(start).Seconds())
	}

	if err := c.reader.Close(); err != nil {
		log.Printf("Error closing Kafka reader: %v", err)
	}
}