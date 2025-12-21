package redis

import (
	"context"
	"messaging-app/config"
	"time"

	"github.com/redis/go-redis/v9"
)

// ClusterClient wraps the Redis cluster client
type ClusterClient struct {
	*redis.ClusterClient
}

func NewClusterClient(cfg *config.Config) *ClusterClient {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        cfg.RedisURLs,
		Password:     cfg.RedisPass,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     200,
		MinIdleConns: 20,
		PoolTimeout:  3 * time.Second,
	})

	return &ClusterClient{client}
}

// Set stores a key-value pair with expiration
func (c *ClusterClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.ClusterClient.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key
func (c *ClusterClient) Get(ctx context.Context, key string) (string, error) {
	return c.ClusterClient.Get(ctx, key).Result()
}

// Del deletes one or more keys
func (c *ClusterClient) Del(ctx context.Context, keys ...string) error {
	return c.ClusterClient.Del(ctx, keys...).Err()
}

// Publish sends a message to a channel
func (c *ClusterClient) Publish(ctx context.Context, channel string, message interface{}) error {
	return c.ClusterClient.Publish(ctx, channel, message).Err()
}

// Subscribe returns a pubsub channel
func (c *ClusterClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.ClusterClient.Subscribe(ctx, channels...)
}

// Close terminates the connection
func (c *ClusterClient) Close() error {
	return c.ClusterClient.Close()
}

// IsAvailable checks if Redis is reachable
func (c *ClusterClient) IsAvailable(ctx context.Context) bool {
	_, err := c.ClusterClient.Ping(ctx).Result()
	return err == nil
}

// GetClient returns the underlying Redis client for advanced operations
func (c *ClusterClient) GetClient() *redis.ClusterClient {
	return c.ClusterClient
}
