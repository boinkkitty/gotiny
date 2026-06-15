package redis

import (
	"context"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"gotiny/key-gen-service/internal/domain"
)

const queueKey = "keygen:pool"

type KeyQueue struct {
	client *goredis.Client
}

func NewKeyQueue(client *goredis.Client) *KeyQueue {
	return &KeyQueue{client: client}
}

func (q *KeyQueue) Pop(ctx context.Context) (string, error) {
	val, err := q.client.LPop(ctx, queueKey).Result()
	if errors.Is(err, goredis.Nil) {
		return "", domain.ErrQueueEmpty
	}
	if err != nil {
		return "", fmt.Errorf("redis lpop: %w", err)
	}
	return val, nil
}

func (q *KeyQueue) PushBatch(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	vals := make([]interface{}, len(keys))
	for i, k := range keys {
		vals[i] = k
	}
	if err := q.client.RPush(ctx, queueKey, vals...).Err(); err != nil {
		return fmt.Errorf("redis rpush: %w", err)
	}
	return nil
}

func (q *KeyQueue) Len(ctx context.Context) (int64, error) {
	n, err := q.client.LLen(ctx, queueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("redis llen: %w", err)
	}
	return n, nil
}
