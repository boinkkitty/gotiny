package service

import (
	"context"
	"errors"
	"testing"

	"gotiny/key-gen-service/internal/domain"
	"gotiny/key-gen-service/internal/port"
)

var _ port.KeyRepository = (*mockRepo)(nil)

type mockRepo struct {
	claimFunc    func(ctx context.Context, instanceID string, batchSize int) ([]string, error)
	countFunc    func(ctx context.Context) (int64, error)
	generateFunc func(ctx context.Context, count int) (int64, error)
}

func (m *mockRepo) ClaimBatch(ctx context.Context, instanceID string, batchSize int) ([]string, error) {
	return m.claimFunc(ctx, instanceID, batchSize)
}

func (m *mockRepo) CountAvailable(ctx context.Context) (int64, error) {
	if m.countFunc != nil {
		return m.countFunc(ctx)
	}
	return 100000, nil
}

func (m *mockRepo) GenerateBatch(ctx context.Context, count int) (int64, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, count)
	}
	return int64(count), nil
}

func TestGetKey_BufferHasKeys(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			keys := make([]string, n)
			for i := range keys {
				keys[i] = "key" + string(rune('A'+i))
			}
			return keys, nil
		},
	}

	svc := NewKeyGenService(repo, Config{
		InstanceID: "test", BufferSize: 10, RefillAt: 2, PoolThreshold: 1000, PoolBatchSize: 100,
	})
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	key, err := svc.GetKey(context.Background())
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if key == "" {
		t.Error("expected non-empty key")
	}
}

func TestGetKey_BufferEmpty_RefillSucceeds(t *testing.T) {
	callCount := 0
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			callCount++
			if callCount == 1 {
				return []string{"key1", "key2"}, nil
			}
			return []string{"refilled1", "refilled2", "refilled3"}, nil
		},
	}

	svc := NewKeyGenService(repo, Config{
		InstanceID: "test", BufferSize: 3, RefillAt: 0, PoolThreshold: 1000, PoolBatchSize: 100,
	})
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	svc.GetKey(context.Background())
	svc.GetKey(context.Background())

	key, err := svc.GetKey(context.Background())
	if err != nil {
		t.Fatalf("get key after refill: %v", err)
	}
	if key == "" {
		t.Error("expected non-empty key after refill")
	}
}

func TestGetKey_BufferEmpty_RefillFails(t *testing.T) {
	callCount := 0
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, _ int) ([]string, error) {
			callCount++
			if callCount == 1 {
				return []string{"onlykey"}, nil
			}
			return nil, errors.New("db connection lost")
		},
	}

	svc := NewKeyGenService(repo, Config{
		InstanceID: "test", BufferSize: 5, RefillAt: 0, PoolThreshold: 1000, PoolBatchSize: 100,
	})
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	svc.GetKey(context.Background())

	_, err := svc.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error when refill fails")
	}
}

func TestGetKey_PoolExhausted(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, _ int) ([]string, error) {
			return nil, nil
		},
	}

	svc := NewKeyGenService(repo, Config{
		InstanceID: "test", BufferSize: 5, RefillAt: 0, PoolThreshold: 1000, PoolBatchSize: 100,
	})

	svc.Init(context.Background())

	_, err := svc.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error when pool exhausted")
	}
	if !errors.Is(err, domain.ErrPoolExhausted) {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
}

func TestInit_RefillFails(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, _ int) ([]string, error) {
			return nil, errors.New("connection refused")
		},
	}

	svc := NewKeyGenService(repo, Config{
		InstanceID: "test", BufferSize: 5, RefillAt: 0, PoolThreshold: 1000, PoolBatchSize: 100,
	})

	err := svc.Init(context.Background())
	if err == nil {
		t.Fatal("expected error from Init when refill fails")
	}
}
