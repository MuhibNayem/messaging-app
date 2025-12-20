package websocket

import "github.com/prometheus/client_golang/prometheus"

var (
	wsConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_connections_total",
		Help: "Current number of active WebSocket connections",
	})
	wsMessagesSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "websocket_messages_sent_total",
		Help: "Total number of messages sent via WebSocket",
	}, []string{"type"})
	pendingDirectMessages = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pending_direct_messages_total",
		Help: "Number of pending direct messages",
	})
	pendingGroupMessages = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pending_group_messages_total",
		Help: "Number of pending group messages",
	})
	broadcastLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "websocket_broadcast_latency_seconds",
		Help:    "Time from message received to send",
		Buckets: prometheus.DefBuckets,
	})
)

func init() {
	prometheus.MustRegister(
		wsConnections,
		wsMessagesSent,
		pendingDirectMessages,
		pendingGroupMessages,
		broadcastLatency,
	)
}
