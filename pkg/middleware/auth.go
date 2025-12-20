package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// AuthMiddleware creates a Gin middleware for JWT authentication with Redis blacklist check
func AuthMiddleware(jwtSecret string, redisClient *redis.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}

		userID, err := ValidateToken(authHeader, jwtSecret, redisClient)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		c.Set("userID", userID)
		c.Next()
	}
}

const wsAuthProtocolName = "connectify.auth"

func WSJwtAuthMiddleware(jwtSecret string, redisClient *redis.ClusterClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := extractWebsocketToken(c.GetHeader("Sec-WebSocket-Protocol"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		userID, err := ValidateToken(tokenString, jwtSecret, redisClient)
		if err != nil {
			fmt.Printf("WS Auth Error: %v\n", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		// Store user ID in context
		c.Set("userID", userID)
		c.Next()
	}
}

func extractWebsocketToken(header string) (string, error) {
	if header == "" {
		return "", fmt.Errorf("Sec-WebSocket-Protocol header required")
	}

	parts := strings.Split(header, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == wsAuthProtocolName {
			continue
		}
		return trimmed, nil
	}
	return "", fmt.Errorf("websocket auth token missing")
}

// ValidateToken validates a JWT token and returns the user ID if valid
// This can be used by both HTTP middleware and WebSocket handlers
func ValidateToken(tokenString, jwtSecret string, redisClient *redis.ClusterClient) (string, error) {
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	if tokenString == "" {
		return "", fmt.Errorf("bearer token required")
	}

	// Check token blacklist
	_, err := redisClient.Get(context.Background(), "blacklist:"+tokenString).Result()
	if err == nil {
		return "", fmt.Errorf("token revoked")
	} else if err != redis.Nil {
		// Only return error if it's not a "key not found" error
		return "", fmt.Errorf("error checking token status")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if claims["type"] != "access" {
			return "", fmt.Errorf("invalid token type")
		}

		userID, ok := claims["id"].(string)
		if !ok {
			return "", fmt.Errorf("invalid token claims")
		}

		return userID, nil
	}

	return "", fmt.Errorf("invalid token")
}

// BlacklistToken adds a token to the Redis blacklist
func BlacklistToken(tokenString string, expiration time.Duration, redisClient *redis.ClusterClient) error {
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	if tokenString == "" {
		return fmt.Errorf("empty token")
	}

	ctx := context.Background()
	return redisClient.Set(ctx, "blacklist:"+tokenString, "1", expiration).Err()
}
