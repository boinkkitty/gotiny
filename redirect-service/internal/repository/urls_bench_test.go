package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"gotiny/redirect-service/internal/repository"
)

func setupBenchDB(b *testing.B) (*pgxpool.Pool, *redis.Client) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, "postgres://postgres:postgres@localhost:5432/gotiny_test?sslmode=disable")
	if err != nil {
		b.Skipf("skipping benchmark: postgres not available: %v", err)
	}

	_, err = pool.Exec(ctx, `
		DROP TABLE IF EXISTS urls;
		CREATE TABLE urls (
			id BIGSERIAL PRIMARY KEY,
			short_code VARCHAR(7) NOT NULL UNIQUE,
			original_url TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		b.Fatalf("setup schema: %v", err)
	}

	for i := 0; i < 1000; i++ {
		code := fmt.Sprintf("bench%02d", i%100)
		if i < 100 {
			pool.Exec(ctx, `INSERT INTO urls (short_code, original_url) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				code, fmt.Sprintf("https://example.com/%d", i))
		}
	}

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(ctx).Err(); err != nil {
		b.Logf("redis not available, benchmarking postgres-only: %v", err)
		return pool, nil
	}
	rdb.FlushDB(ctx)

	b.Cleanup(func() {
		pool.Close()
		if rdb != nil {
			rdb.Close()
		}
	})

	return pool, rdb
}

func BenchmarkResolve_PostgresOnly(b *testing.B) {
	pool, _ := setupBenchDB(b)
	repo := repository.NewURLRepository(pool, nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		code := fmt.Sprintf("bench%02d", i%100)
		_, err := repo.Resolve(ctx, code)
		if err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}

func BenchmarkResolve_RedisCacheHit(b *testing.B) {
	pool, rdb := setupBenchDB(b)
	if rdb == nil {
		b.Skip("redis not available")
	}
	repo := repository.NewURLRepository(pool, rdb)
	ctx := context.Background()

	// Warm the cache
	for i := 0; i < 100; i++ {
		code := fmt.Sprintf("bench%02d", i)
		repo.Resolve(ctx, code)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		code := fmt.Sprintf("bench%02d", i%100)
		_, err := repo.Resolve(ctx, code)
		if err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}

func BenchmarkResolve_RedisCacheMiss(b *testing.B) {
	pool, rdb := setupBenchDB(b)
	if rdb == nil {
		b.Skip("redis not available")
	}
	repo := repository.NewURLRepository(pool, rdb)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rdb.FlushDB(ctx)
		code := fmt.Sprintf("bench%02d", i%100)
		_, err := repo.Resolve(ctx, code)
		if err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}
