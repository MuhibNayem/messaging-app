package config

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	HTTPRequests          *prometheus.CounterVec
	HTTPDuration         *prometheus.HistogramVec
	WebsocketConnections prometheus.Gauge
	KafkaMessages        *prometheus.CounterVec
	HTTPErrors           *prometheus.CounterVec
}

var (
	metricsInstance *Metrics
	metricsOnce     sync.Once
)

// GetMetrics returns a singleton instance of Metrics
func GetMetrics() *Metrics {
	metricsOnce.Do(func() {
		metricsInstance = &Metrics{
			HTTPRequests: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "messaging_http_requests_total",
					Help: "Count of HTTP requests",
				},
				[]string{"method", "path", "status"},
			),
			HTTPDuration: prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "messaging_http_request_duration_seconds",
					Help:    "Duration of HTTP requests",
					Buckets: []float64{0.1, 0.3, 0.5, 1, 3, 5, 10},
				},
				[]string{"method", "path"},
			),
			WebsocketConnections: prometheus.NewGauge(
				prometheus.GaugeOpts{
					Name: "messaging_websocket_connections",
					Help: "Current WebSocket connections",
				},
			),
			KafkaMessages: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "messaging_kafka_messages_total",
					Help: "Count of Kafka messages",
				},
				[]string{"topic", "type"},
			),
			HTTPErrors: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "messaging_http_errors_total",
					Help: "Count of HTTP errors",
				},
				[]string{"method", "path", "status"},
			),
		}

		prometheus.MustRegister(
			metricsInstance.HTTPRequests,
			metricsInstance.HTTPDuration,
			metricsInstance.WebsocketConnections,
			metricsInstance.KafkaMessages,
			metricsInstance.HTTPErrors,
		)
	})
	return metricsInstance
}

// responseRecorder wraps http.ResponseWriter to capture status code
type responseRecorder struct {
	gin.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// MetricsMiddleware captures HTTP metrics for Gin routes
func MetricsMiddleware(metrics *Metrics) gin.HandlerFunc {	
	return func(c *gin.Context) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: c.Writer, statusCode: http.StatusOK}
		c.Writer = recorder

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(recorder.statusCode)

		metrics.HTTPRequests.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			status,
		).Inc()

		metrics.HTTPDuration.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
		).Observe(duration)

		if recorder.statusCode >= 400 {
			metrics.HTTPErrors.WithLabelValues(
				c.Request.Method,
				c.FullPath(),
				status,
			).Inc()
		}
	}
}

// MetricsHandler returns the Prometheus metrics handler
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// IncWebsocketConnections increments the WebSocket connections gauge
func IncWebsocketConnections(metrics *Metrics) {
	metrics.WebsocketConnections.Inc()
}

// DecWebsocketConnections decrements the WebSocket connections gauge
func DecWebsocketConnections(metrics *Metrics) {
	metrics.WebsocketConnections.Dec()
}

// RecordKafkaMessage records a Kafka message in metrics
func RecordKafkaMessage(metrics Metrics, topic, msgType string) {
	metrics.KafkaMessages.WithLabelValues(topic, msgType).Inc()
}