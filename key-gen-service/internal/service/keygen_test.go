package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gotiny/key-gen-service/internal/domain"
	"gotiny/key-gen-service/internal/port"
)

var _ port.KeyRepository = (*mockRepo)(nil)

type mockRepo struct {
	claimFunc     func(ctx context.Context, instanceID string, batchSize int) ([]string, error)
	countFunc     func(ctx context.Context) (int64, error)
	generateFunc  func(ctx context.Context, count int) (int64, error)
	reclaimFunc   func(ctx context.Context, olderThan time.Duration) (int64, error)
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

func (m *mockRepo) ReclaimOrphaned(ctx context.Context, olderThan time.Duration) (int64, error) {
	if m.reclaimFunc != nil {
		return m.reclaimFunc(ctx, olderThan)
	}
	return 0, nil
}

var _ port.KeyQueue = (*mockQueue)(nil)

type mockQueue struct {
	popFunc  func(ctx context.Context) (string, error)
	pushFunc func(ctx context.Context, keys []string) error
	lenFunc  func(ctx context.Context) (int64, error)
}

func (m *mockQueue) Pop(ctx context.Context) (string, error) {
	if m.popFunc != nil {
		return m.popFunc(ctx)
	}
	return "", domain.ErrQueueEmpty
}

func (m *mockQueue) PushBatch(ctx context.Context, keys []string) error {
	if m.pushFunc != nil {
		return m.pushFunc(ctx, keys)
	}
	return nil
}

func (m *mockQueue) Len(ctx context.Context) (int64, error) {
	if m.lenFunc != nil {
		return m.lenFunc(ctx)
	}
	return 0, nil
}

var _ port.RefillLock = (*mockLock)(nil)

type mockLock struct {
	acquireFunc func(ctx context.Context, threshold int64) (bool, error)
}

func (m *mockLock) AcquireIfBelow(ctx context.Context, threshold int64) (bool, error) {
	if m.acquireFunc != nil {
		return m.acquireFunc(ctx, threshold)
	}
	return false, nil
}

func defaultCfg() Config {
	return Config{
		InstanceID:      "test",
		BufferSize:      10,
		RefillAt:        2,
		PoolThreshold:   1000,
		PoolBatchSize:   100,
		QueueLowWater:   5,
		QueueHighWater:  20,
		QueueRefillTick: time.Hour,
		CBMaxFailures:   3,
		CBResetTimeout:  50 * time.Millisecond,
		LockTTL:         60 * time.Second,
		ReconcileTick:   time.Hour,
		ReconcileMaxAge: time.Hour,
	}
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

	svc := NewKeyGenService(repo, nil, nil, defaultCfg())
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

	cfg := defaultCfg()
	cfg.BufferSize = 3
	cfg.RefillAt = 0

	svc := NewKeyGenService(repo, nil, nil, cfg)
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

	cfg := defaultCfg()
	cfg.BufferSize = 5
	cfg.RefillAt = 0

	svc := NewKeyGenService(repo, nil, nil, cfg)
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

	cfg := defaultCfg()
	cfg.BufferSize = 5
	cfg.RefillAt = 0

	svc := NewKeyGenService(repo, nil, nil, cfg)
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

	cfg := defaultCfg()
	cfg.BufferSize = 5
	cfg.RefillAt = 0

	svc := NewKeyGenService(repo, nil, nil, cfg)
	err := svc.Init(context.Background())
	if err == nil {
		t.Fatal("expected error from Init when refill fails")
	}
}

func TestGetKey_RedisLPOP_Success(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			return make([]string, n), nil
		},
	}
	queue := &mockQueue{
		popFunc: func(_ context.Context) (string, error) {
			return "redis-key-1", nil
		},
		lenFunc: func(_ context.Context) (int64, error) {
			return 1000, nil
		},
	}

	svc := NewKeyGenService(repo, queue, &mockLock{}, defaultCfg())
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	key, err := svc.GetKey(context.Background())
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if key != "redis-key-1" {
		t.Errorf("expected redis-key-1, got %q", key)
	}
}

func TestGetKey_RedisEmpty_SyncRefillAndPop(t *testing.T) {
	popCount := 0
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			return []string{"claimed1", "claimed2"}, nil
		},
	}
	queue := &mockQueue{
		popFunc: func(_ context.Context) (string, error) {
			popCount++
			if popCount == 1 {
				return "", domain.ErrQueueEmpty
			}
			return "after-refill", nil
		},
		pushFunc: func(_ context.Context, keys []string) error {
			return nil
		},
		lenFunc: func(_ context.Context) (int64, error) {
			return 0, nil
		},
	}
	lock := &mockLock{
		acquireFunc: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
	}

	svc := NewKeyGenService(repo, queue, lock, defaultCfg())
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	key, err := svc.GetKey(context.Background())
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if key != "after-refill" {
		t.Errorf("expected after-refill, got %q", key)
	}
}

func TestGetKey_RedisError_FallsBackToBuffer(t *testing.T) {
	failCount := 0
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			keys := make([]string, n)
			for i := range keys {
				keys[i] = "buf-key"
			}
			return keys, nil
		},
	}
	queue := &mockQueue{
		popFunc: func(_ context.Context) (string, error) {
			failCount++
			return "", errors.New("redis timeout")
		},
		lenFunc: func(_ context.Context) (int64, error) {
			return 1000, nil
		},
	}

	svc := NewKeyGenService(repo, queue, &mockLock{}, defaultCfg())
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	key, err := svc.GetKey(context.Background())
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if key != "buf-key" {
		t.Errorf("expected buf-key, got %q", key)
	}
}

func TestGetKey_CircuitOpen_UsesBuffer(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			keys := make([]string, n)
			for i := range keys {
				keys[i] = "buffer-key"
			}
			return keys, nil
		},
	}
	queue := &mockQueue{
		popFunc: func(_ context.Context) (string, error) {
			return "", errors.New("redis down")
		},
		lenFunc: func(_ context.Context) (int64, error) {
			return 0, errors.New("redis down")
		},
	}

	cfg := defaultCfg()
	cfg.CBMaxFailures = 2

	svc := NewKeyGenService(repo, queue, &mockLock{}, cfg)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Trip the circuit breaker
	svc.GetKey(context.Background())
	svc.GetKey(context.Background())

	if svc.cb.State() != stateOpen {
		t.Fatalf("expected circuit open, got %d", svc.cb.State())
	}

	key, err := svc.GetKey(context.Background())
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if key != "buffer-key" {
		t.Errorf("expected buffer-key, got %q", key)
	}
}

func TestGetKey_CircuitHalfOpen_Success(t *testing.T) {
	popCalls := 0
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			return make([]string, n), nil
		},
	}
	queue := &mockQueue{
		popFunc: func(_ context.Context) (string, error) {
			popCalls++
			if popCalls <= 2 {
				return "", errors.New("redis down")
			}
			return "recovered-key", nil
		},
		lenFunc: func(_ context.Context) (int64, error) {
			return 1000, nil
		},
	}

	cfg := defaultCfg()
	cfg.CBMaxFailures = 2
	cfg.CBResetTimeout = 100 * time.Millisecond

	svc := NewKeyGenService(repo, queue, &mockLock{}, cfg)
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	svc.GetKey(context.Background())
	svc.GetKey(context.Background())

	time.Sleep(150 * time.Millisecond)

	key, err := svc.GetKey(context.Background())
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if key != "recovered-key" {
		t.Errorf("expected recovered-key, got %q", key)
	}
	if svc.cb.State() != stateClosed {
		t.Fatalf("expected circuit closed after success, got %d", svc.cb.State())
	}
}

func TestInit_RedisDown_StartsWithCircuitOpen(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			return make([]string, n), nil
		},
	}
	queue := &mockQueue{
		lenFunc: func(_ context.Context) (int64, error) {
			return 0, errors.New("redis down")
		},
	}

	svc := NewKeyGenService(repo, queue, &mockLock{}, defaultCfg())
	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	if svc.cb.State() != stateOpen {
		t.Fatalf("expected circuit open when Redis down at init, got %d", svc.cb.State())
	}
}
