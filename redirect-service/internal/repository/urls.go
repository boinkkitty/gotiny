package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
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
		if err := r.redis.Set(ctx, cacheKey(shortCode), originalURL, 24*time.Hour).Err(); err != nil {
			slog.Warn("redis set failed", "error", err)
		}
	}

	return originalURL, nil
}

func cacheKey(shortCode string) string {
	return "url:" + shortCode
}
