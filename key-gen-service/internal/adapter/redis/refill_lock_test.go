package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestAcquireIfBelow_LockFreeAndBelowThreshold(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	mr.RPush(queueKey, "a", "b")

	lock := NewRefillLock(client, "inst-1", 60*time.Second)
	acquired, err := lock.AcquireIfBelow(context.Background(), 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !acquired {
		t.Fatal("expected to acquire lock")
	}

	if !mr.Exists(lockKey) {
		t.Fatal("expected lock key to exist")
	}
	val, _ := mr.Get(lockKey)
	if val != "inst-1" {
		t.Errorf("expected lock value inst-1, got %q", val)
	}
}

func TestAcquireIfBelow_LockAlreadyHeld(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	mr.RPush(queueKey, "a")
	mr.Set(lockKey, "other-instance")

	lock := NewRefillLock(client, "inst-1", 60*time.Second)
	acquired, err := lock.AcquireIfBelow(context.Background(), 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acquired {
		t.Fatal("should not acquire when lock is held")
	}
}

func TestAcquireIfBelow_AboveThreshold(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	for i := 0; i < 600; i++ {
		mr.RPush(queueKey, "k")
	}

	lock := NewRefillLock(client, "inst-1", 60*time.Second)
	acquired, err := lock.AcquireIfBelow(context.Background(), 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acquired {
		t.Fatal("should not acquire when above threshold")
	}
}
