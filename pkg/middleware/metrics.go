package middleware

import (
	"messaging-app/config"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()

		c.Next()

		duration := time.Since(start).Seconds()
		status := c.Writer.Status()

		config.HTTPRequests.WithLabelValues(c.Request.Method, path, strconv.Itoa(status)).Inc()
		config.HTTPDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}