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
	defer c.reader.Close()

	for {
		start := time.Now()
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Error reading message: %v", err)
			continue
		}

		var message models.Message
		if err := json.Unmarshal(msg.Value, &message); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		// Broadcast to WebSocket clients
		c.hub.Broadcast <- message

		messagesConsumed.WithLabelValues(c.reader.Config().Topic).Inc()
		consumeDuration.WithLabelValues(c.reader.Config().Topic).Observe(time.Since(start).Seconds())
	}
}