package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	baseCacheTTL  = 6 * time.Hour
	jitterMaxSecs = 60
)

var ErrNotFound = errors.New("short code not found")

type URLRepository struct {
	pool  *pgxpool.Pool
	redis *redis.Client
}

func NewURLRepository(pool *pgxpool.Pool, redis *redis.Client) *URLRepository {
	return &URLRepository{pool: pool, redis: redis}
}

func (r *URLRepository) Resolve(ctx context.Context, shortCode string) (string, error) {
	if r.redis != nil {
		cached, err := r.redis.Get(ctx, cacheKey(shortCode)).Result()
		if err == nil {
			slog.Debug("cache hit", "short_code", shortCode)
			return cached, nil
		}
		if !errors.Is(err, redis.Nil) {
			slog.Warn("redis get failed, falling back to postgres", "error", err)
		}
	}

	var originalURL string
	err := r.pool.QueryRow(ctx,
		`SELECT original_url FROM urls WHERE short_code = $1`, shortCode,
	).Scan(&originalURL)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query url: %w", err)
	}

	if r.redis != nil {
		ttl := baseCacheTTL + time.Duration(rand.IntN(jitterMaxSecs))*time.Second
		if err := r.redis.Set(ctx, cacheKey(shortCode), originalURL, ttl).Err(); err != nil {
			slog.Warn("redis set failed", "error", err)
		}
	}

	return originalURL, nil
}

func (r *URLRepository) InvalidateCache(ctx context.Context, shortCode string) error {
	if r.redis == nil {
		return nil
	}
	if err := r.redis.Del(ctx, cacheKey(shortCode)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

func cacheKey(shortCode string) string {
	return "url:" + shortCode
}
