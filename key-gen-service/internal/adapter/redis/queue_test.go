package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"gotiny/key-gen-service/internal/domain"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return mr, client
}

func TestPop_Success(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.RPush(queueKey, "key1", "key2", "key3")

	q := NewKeyQueue(client)
	val, err := q.Pop(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "key1" {
		t.Errorf("expected key1, got %q", val)
	}
}

func TestPop_Empty(t *testing.T) {
	_, client := setupMiniredis(t)

	q := NewKeyQueue(client)
	_, err := q.Pop(context.Background())
	if err != domain.ErrQueueEmpty {
		t.Fatalf("expected ErrQueueEmpty, got %v", err)
	}
}

func TestPop_Error(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.Close()

	q := NewKeyQueue(client)
	_, err := q.Pop(context.Background())
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
}

func TestPushBatch_Success(t *testing.T) {
	mr, client := setupMiniredis(t)

	q := NewKeyQueue(client)
	err := q.PushBatch(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vals, err := mr.List(queueKey)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(vals) != 3 {
		t.Errorf("expected 3 items, got %d", len(vals))
	}
}

func TestPushBatch_Empty(t *testing.T) {
	_, client := setupMiniredis(t)

	q := NewKeyQueue(client)
	err := q.PushBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPushBatch_Error(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.Close()

	q := NewKeyQueue(client)
	err := q.PushBatch(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
}

func TestLen_Success(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.RPush(queueKey, "a", "b")

	q := NewKeyQueue(client)
	n, err := q.Len(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
}

func TestLen_Error(t *testing.T) {
	mr, client := setupMiniredis(t)
	mr.Close()

	q := NewKeyQueue(client)
	_, err := q.Len(context.Background())
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
}
