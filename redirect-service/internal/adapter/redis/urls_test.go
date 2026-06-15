package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"gotiny/redirect-service/internal/domain"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return mr, client
}

func TestGet_Hit(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.Set(cacheKey("abc"), "https://example.com")

	cache := NewURLCache(client)
	val, err := cache.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "https://example.com" {
		t.Errorf("expected https://example.com, got %q", val)
	}
}

func TestGet_Miss(t *testing.T) {
	_, client := setupMiniredis(t)

	cache := NewURLCache(client)
	_, err := cache.Get(context.Background(), "missing")
	if err != domain.ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss, got %v", err)
	}
}

func TestGet_Error(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.Close()

	cache := NewURLCache(client)
	_, err := cache.Get(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
	if err == domain.ErrCacheMiss {
		t.Fatal("expected connection error, not cache miss")
	}
}

func TestSet_Success(t *testing.T) {
	mr, client := setupMiniredis(t)

	cache := NewURLCache(client)
	if err := cache.Set(context.Background(), "abc", "https://example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := mr.Get(cacheKey("abc"))
	if err != nil {
		t.Fatalf("get from miniredis: %v", err)
	}
	if val != "https://example.com" {
		t.Errorf("expected https://example.com, got %q", val)
	}

	ttl := mr.TTL(cacheKey("abc"))
	if ttl < baseCacheTTL || ttl > baseCacheTTL+jitterMaxSecs*1e9 {
		t.Errorf("TTL %v outside expected range [%v, %v]", ttl, baseCacheTTL, baseCacheTTL+jitterMaxSecs*1e9)
	}
}

func TestSet_Error(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.Close()

	cache := NewURLCache(client)
	err := cache.Set(context.Background(), "abc", "https://example.com")
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
}

func TestDelete_Success(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.Set(cacheKey("abc"), "https://example.com")

	cache := NewURLCache(client)
	if err := cache.Delete(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mr.Exists(cacheKey("abc")) {
		t.Error("expected key to be deleted")
	}
}
