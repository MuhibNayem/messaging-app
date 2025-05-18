package kafka

import (
	"context"
	"encoding/json"
	"messaging-app/internal/models"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
)

var (
	messagesProduced = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_messages_produced_total",
			Help: "Total number of messages produced to Kafka",
		},
		[]string{"topic"},
	)
	produceDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kafka_produce_duration_seconds",
			Help:    "Duration of Kafka produce operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic"},
	)
)

type MessageProducer struct {
	writer *kafka.Writer
	topic  string
}

func NewMessageProducer(brokers []string, topic string) *MessageProducer {
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		BatchSize:    1000,
		BatchBytes:   4 * 1024 * 1024, // 4MB
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
		Compression:  compress.Snappy,
		Async:        true,
		Completion: func(messages []kafka.Message, err error) {
			if err == nil {
				messagesProduced.WithLabelValues(topic).Add(float64(len(messages)))
			}
		},
	}

	return &MessageProducer{
		writer: w,
		topic:  topic,
	}
}

func (p *MessageProducer) ProduceMessage(ctx context.Context, message models.Message) error {
	start := time.Now()
	defer func() {
		produceDuration.WithLabelValues(p.topic).Observe(time.Since(start).Seconds())
	}()

	jsonMsg, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return p.writer.WriteMessages(ctx,
		kafka.Message{
			Key:   []byte(message.ReceiverID.Hex()),
			Value: jsonMsg,
			Time:  time.Now(),
		},
	)
}

func (p *MessageProducer) Close() error {
	return p.writer.Close()
}