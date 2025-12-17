package middleware

import (
	"net/http"
	"sync"

	"messaging-app/config"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// IPRateLimiter stores a rate limiter for each IP address
type IPRateLimiter struct {
	ips   map[string]*rate.Limiter
	mu    *sync.RWMutex
	limit rate.Limit
	burst int
}

// NewIPRateLimiter creates a new IPRateLimiter
func NewIPRateLimiter(limit rate.Limit, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		ips:   make(map[string]*rate.Limiter),
		mu:    &sync.RWMutex{},
		limit: limit,
		burst: burst,
	}
}

// AddIP creates a new rate limiter for an IP address
func (i *IPRateLimiter) AddIP(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter := rate.NewLimiter(i.limit, i.burst)
	i.ips[ip] = limiter
	return limiter
}

// GetLimiter returns the rate limiter for an IP address
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.RLock()
	limiter, exists := i.ips[ip]
	i.mu.RUnlock()

	if !exists {
		return i.AddIP(ip)
	}

	return limiter
}

// RateLimiter is a middleware that limits the number of requests per IP
func RateLimiter(cfg *config.Config) gin.HandlerFunc {
	if !cfg.RateLimitEnabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	limiter := NewIPRateLimiter(rate.Limit(cfg.RateLimitLimit), cfg.RateLimitBurst)

	return func(c *gin.Context) {
		ipLimiter := limiter.GetLimiter(c.ClientIP())
		if !ipLimiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}

		c.Next()
	}
}

// StrictRateLimiter creates a custom rate limiter middleware with specified limit and burst
func StrictRateLimiter(r float64, b int) gin.HandlerFunc {
	limiter := NewIPRateLimiter(rate.Limit(r), b)

	return func(c *gin.Context) {
		ipLimiter := limiter.GetLimiter(c.ClientIP())
		if !ipLimiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests. Please slow down."})
			return
		}

		c.Next()
	}
}
