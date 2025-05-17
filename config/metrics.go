package config

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Count of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: []float64{0.1, 0.3, 0.5, 1, 3, 5, 10},
		},
		[]string{"method", "path"},
	)

	WebsocketConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "websocket_connections",
			Help: "Current WebSocket connections",
		},
	)

	KafkaMessages = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_messages_total",
			Help: "Count of Kafka messages",
		},
		[]string{"topic", "type"},
	)
)

func InitMetrics() {
	prometheus.MustRegister(
		HTTPRequests,
		HTTPDuration,
		WebsocketConnections,
		KafkaMessages,
	)
}

func MetricsHandler() http.Handler {
	return promhttp.Handler()
}