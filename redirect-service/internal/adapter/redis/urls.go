package redis

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	baseCacheTTL  = 6 * time.Hour
	jitterMaxSecs = 60
)

type URLCache struct {
	client *goredis.Client
}

func NewURLCache(client *goredis.Client) *URLCache {
	return &URLCache{client: client}
}

func (c *URLCache) Get(ctx context.Context, shortCode string) (string, error) {
	val, err := c.client.Get(ctx, cacheKey(shortCode)).Result()
	if errors.Is(err, goredis.Nil) {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("redis get: %w", err)
	}
	return val, nil
}

func (c *URLCache) Set(ctx context.Context, shortCode, originalURL string) error {
	ttl := baseCacheTTL + time.Duration(rand.IntN(jitterMaxSecs))*time.Second
	if err := c.client.Set(ctx, cacheKey(shortCode), originalURL, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

func (c *URLCache) Delete(ctx context.Context, shortCode string) error {
	if err := c.client.Del(ctx, cacheKey(shortCode)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

func cacheKey(shortCode string) string {
	return "url:" + shortCode
}
