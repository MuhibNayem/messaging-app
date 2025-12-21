package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.com/spydotech-group/shared-entity/models"
	"gitlab.com/spydotech-group/shared-entity/redis"
)

// EventCache provides caching for event-related data
type EventCache struct {
	client *redis.ClusterClient
}

// Cache TTL constants
const (
	EventStatsTTL      = 60 * time.Second // Event stats (going/interested counts)
	EventBasicTTL      = 5 * time.Minute  // Basic event data
	UserRSVPStatusTTL  = 5 * time.Minute  // User's RSVP status for an event
	FriendsGoingTTL    = 2 * time.Minute  // Friends going to an event
	TrendingEventsTTL  = 5 * time.Minute  // Trending events list
	EventCategoriesTTL = 1 * time.Hour    // Categories with counts
)

// NewEventCache creates a new event cache instance
func NewEventCache(client *redis.ClusterClient) *EventCache {
	if client == nil {
		return nil
	}
	return &EventCache{client: client}
}

// Key builders
func eventStatsKey(eventID string) string {
	return fmt.Sprintf("event:%s:stats", eventID)
}

func userRSVPStatusKey(userID, eventID string) string {
	return fmt.Sprintf("user:%s:event:%s:status", userID, eventID)
}

func friendsGoingKey(userID, eventID string) string {
	return fmt.Sprintf("user:%s:event:%s:friends_going", userID, eventID)
}

func trendingEventsKey() string {
	return "events:trending"
}

func categoriesKey() string {
	return "events:categories"
}

// EventStats represents cached event statistics
type EventStats struct {
	GoingCount      int64 `json:"going_count"`
	InterestedCount int64 `json:"interested_count"`
	InvitedCount    int64 `json:"invited_count"`
}

// GetEventStats retrieves cached event stats
func (c *EventCache) GetEventStats(ctx context.Context, eventID string) (*EventStats, error) {
	if c == nil {
		return nil, nil
	}

	data, err := c.client.Get(ctx, eventStatsKey(eventID))
	if err != nil || data == "" {
		return nil, nil // Cache miss
	}

	var stats EventStats
	if err := json.Unmarshal([]byte(data), &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// SetEventStats caches event stats
func (c *EventCache) SetEventStats(ctx context.Context, eventID string, stats *models.EventStats) error {
	if c == nil {
		return nil
	}

	data, err := json.Marshal(EventStats{
		GoingCount:      stats.GoingCount,
		InterestedCount: stats.InterestedCount,
		InvitedCount:    stats.InvitedCount,
	})
	if err != nil {
		return err
	}

	return c.client.Set(ctx, eventStatsKey(eventID), data, EventStatsTTL)
}

// InvalidateEventStats removes cached event stats
func (c *EventCache) InvalidateEventStats(ctx context.Context, eventID string) error {
	if c == nil {
		return nil
	}
	return c.client.Del(ctx, eventStatsKey(eventID))
}

// GetUserRSVPStatus retrieves cached RSVP status
func (c *EventCache) GetUserRSVPStatus(ctx context.Context, userID, eventID string) (models.RSVPStatus, bool) {
	if c == nil {
		return "", false
	}

	status, err := c.client.Get(ctx, userRSVPStatusKey(userID, eventID))
	if err != nil || status == "" {
		return "", false
	}
	return models.RSVPStatus(status), true
}

// SetUserRSVPStatus caches user's RSVP status
func (c *EventCache) SetUserRSVPStatus(ctx context.Context, userID, eventID string, status models.RSVPStatus) error {
	if c == nil {
		return nil
	}
	return c.client.Set(ctx, userRSVPStatusKey(userID, eventID), string(status), UserRSVPStatusTTL)
}

// InvalidateUserRSVPStatus removes cached RSVP status
func (c *EventCache) InvalidateUserRSVPStatus(ctx context.Context, userID, eventID string) error {
	if c == nil {
		return nil
	}
	return c.client.Del(ctx, userRSVPStatusKey(userID, eventID))
}

// GetFriendsGoing retrieves cached friends going list
func (c *EventCache) GetFriendsGoing(ctx context.Context, userID, eventID string) ([]string, error) {
	if c == nil {
		return nil, nil
	}

	data, err := c.client.Get(ctx, friendsGoingKey(userID, eventID))
	if err != nil || data == "" {
		return nil, nil // Cache miss
	}

	var friendIDs []string
	if err := json.Unmarshal([]byte(data), &friendIDs); err != nil {
		return nil, err
	}
	return friendIDs, nil
}

// SetFriendsGoing caches friends going list
func (c *EventCache) SetFriendsGoing(ctx context.Context, userID, eventID string, friendIDs []string) error {
	if c == nil {
		return nil
	}

	data, err := json.Marshal(friendIDs)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, friendsGoingKey(userID, eventID), data, FriendsGoingTTL)
}

// InvalidateFriendsGoing removes cached friends going for a specific user/event
func (c *EventCache) InvalidateFriendsGoing(ctx context.Context, userID, eventID string) error {
	if c == nil {
		return nil
	}
	return c.client.Del(ctx, friendsGoingKey(userID, eventID))
}

// InvalidateAllEventCache invalidates all cache for an event
func (c *EventCache) InvalidateAllEventCache(ctx context.Context, eventID string) error {
	if c == nil {
		return nil
	}

	// Invalidate stats
	c.InvalidateEventStats(ctx, eventID)

	return nil
}

// GetTrendingEvents retrieves cached trending event IDs
func (c *EventCache) GetTrendingEvents(ctx context.Context) ([]string, error) {
	if c == nil {
		return nil, nil
	}

	data, err := c.client.Get(ctx, trendingEventsKey())
	if err != nil || data == "" {
		return nil, nil
	}

	var eventIDs []string
	if err := json.Unmarshal([]byte(data), &eventIDs); err != nil {
		return nil, err
	}
	return eventIDs, nil
}

// SetTrendingEvents caches trending event IDs
func (c *EventCache) SetTrendingEvents(ctx context.Context, eventIDs []string) error {
	if c == nil {
		return nil
	}

	data, err := json.Marshal(eventIDs)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, trendingEventsKey(), data, TrendingEventsTTL)
}

// GetCategories returns cached event categories.
func (c *EventCache) GetCategories(ctx context.Context) ([]models.EventCategory, error) {
	if c == nil {
		return nil, nil
	}

	data, err := c.client.Get(ctx, categoriesKey())
	if err != nil || data == "" {
		return nil, nil
	}

	var categories []models.EventCategory
	if err := json.Unmarshal([]byte(data), &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// SetCategories caches event categories with counts.
func (c *EventCache) SetCategories(ctx context.Context, categories []models.EventCategory) error {
	if c == nil {
		return nil
	}
	data, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, categoriesKey(), data, EventCategoriesTTL)
}
