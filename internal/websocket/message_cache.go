package websocket

import (
	"context"
	"encoding/json"
	"time"

	"gitlab.com/spydotech-group/shared-entity/models"
	"gitlab.com/spydotech-group/shared-entity/redis"
)

// MessageCache handles storing and retrieving messages and pending queues.
type MessageCache struct {
	redis *redis.ClusterClient
}

func NewMessageCache(redisClient *redis.ClusterClient) *MessageCache {
	return &MessageCache{redis: redisClient}
}

func (mc *MessageCache) Store(ctx context.Context, msg models.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	key := "msg:" + msg.ID.Hex()
	if err := mc.redis.Set(ctx, key, data, 24*time.Hour); err != nil {
		return err
	}
	if !msg.ReceiverID.IsZero() {
		return mc.AddPendingDirectMessage(ctx, msg.ReceiverID.Hex(), msg.ID.Hex())
	}
	if !msg.GroupID.IsZero() {
		return mc.AddPendingGroupMessage(ctx, msg.GroupID.Hex(), msg.ID.Hex())
	}
	return nil
}

func (mc *MessageCache) Get(ctx context.Context, msgID string) (*models.Message, error) {
	data, err := mc.redis.Get(ctx, "msg:"+msgID)
	if err != nil {
		return nil, err
	}
	var m models.Message
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (mc *MessageCache) AddPendingDirectMessage(ctx context.Context, userID, msgID string) error {
	return mc.redis.SAdd(ctx, "pending:direct:"+userID, msgID).Err()
}

func (mc *MessageCache) GetPendingDirectMessages(ctx context.Context, userID string) ([]string, error) {
	return mc.redis.SMembers(ctx, "pending:direct:"+userID).Result()
}

func (mc *MessageCache) RemovePendingDirectMessage(ctx context.Context, userID, msgID string) error {
	return mc.redis.SRem(ctx, "pending:direct:"+userID, msgID).Err()
}

func (mc *MessageCache) AddPendingGroupMessage(ctx context.Context, groupID, msgID string) error {
	return mc.redis.SAdd(ctx, "pending:group:"+groupID, msgID).Err()
}

func (mc *MessageCache) GetPendingGroupMessages(ctx context.Context, groupID string) ([]string, error) {
	return mc.redis.SMembers(ctx, "pending:group:"+groupID).Result()
}

func (mc *MessageCache) RemovePendingGroupMessage(ctx context.Context, groupID, msgID string) error {
	return mc.redis.SRem(ctx, "pending:group:"+groupID, msgID).Err()
}
