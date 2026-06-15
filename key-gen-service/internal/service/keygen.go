package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gotiny/key-gen-service/internal/domain"
	"gotiny/key-gen-service/internal/port"
)

type KeyGenService struct {
	repo       port.KeyRepository
	queue      port.KeyQueue
	refillLock port.RefillLock
	instanceID string

	mu     sync.Mutex
	buffer []string

	cb          *CircuitBreaker
	refilling   atomic.Bool
	stopReconcile chan struct{}

	bufferSize       int
	refillAt         int
	poolThreshold    int64
	poolBatchSize    int
	queueLowWater    int64
	queueHighWater   int64
	queueRefillTick  time.Duration
	reconcileTick    time.Duration
	reconcileMaxAge  time.Duration
}

type Config struct {
	InstanceID    string
	BufferSize    int
	RefillAt      int
	PoolThreshold int64
	PoolBatchSize int

	QueueLowWater    int64
	QueueHighWater   int64
	QueueRefillTick  time.Duration
	CBMaxFailures    int
	CBResetTimeout   time.Duration
	LockTTL          time.Duration
	ReconcileTick    time.Duration
	ReconcileMaxAge  time.Duration
}

func DefaultConfig(instanceID string) Config {
	return Config{
		InstanceID:      instanceID,
		BufferSize:      100,
		RefillAt:        20,
		PoolThreshold:   10000,
		PoolBatchSize:   50000,
		QueueLowWater:   500,
		QueueHighWater:  2000,
		QueueRefillTick: 5 * time.Second,
		CBMaxFailures:   5,
		CBResetTimeout:  30 * time.Second,
		LockTTL:         60 * time.Second,
		ReconcileTick:   10 * time.Minute,
		ReconcileMaxAge: 1 * time.Hour,
	}
}

func NewKeyGenService(repo port.KeyRepository, queue port.KeyQueue, lock port.RefillLock, cfg Config) *KeyGenService {
	var cb *CircuitBreaker
	if queue != nil {
		cb = NewCircuitBreaker(cfg.CBMaxFailures, cfg.CBResetTimeout)
	}

	return &KeyGenService{
		repo:            repo,
		queue:           queue,
		refillLock:      lock,
		instanceID:      cfg.InstanceID,
		cb:              cb,
		stopReconcile:   make(chan struct{}),
		bufferSize:      cfg.BufferSize,
		refillAt:        cfg.RefillAt,
		poolThreshold:   cfg.PoolThreshold,
		poolBatchSize:   cfg.PoolBatchSize,
		queueLowWater:   cfg.QueueLowWater,
		queueHighWater:  cfg.QueueHighWater,
		queueRefillTick: cfg.QueueRefillTick,
		reconcileTick:   cfg.ReconcileTick,
		reconcileMaxAge: cfg.ReconcileMaxAge,
	}
}

func (s *KeyGenService) Init(ctx context.Context) error {
	if s.queue != nil {
		if err := s.fillQueue(ctx); err != nil {
			slog.Warn("initial queue fill failed, starting with circuit open", "error", err)
			s.cb.SetOpen()
		}
	}

	if err := s.refillBuffer(ctx); err != nil {
		return fmt.Errorf("initial buffer fill: %w", err)
	}

	go s.poolReplenishLoop(ctx)

	if s.queue != nil {
		go s.queueRefillLoop(ctx)
	}

	go s.reconcileLoop(ctx)

	slog.Info("key-gen service initialized",
		"instance_id", s.instanceID,
		"buffer_size", len(s.buffer),
		"redis_enabled", s.queue != nil,
	)
	return nil
}

func (s *KeyGenService) GetKey(ctx context.Context) (string, error) {
	if s.queue != nil && s.cb.Allow() {
		popCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		key, err := s.queue.Pop(popCtx)
		cancel()

		if err == nil {
			s.cb.RecordSuccess()
			s.maybeReactiveRefill()
			return key, nil
		}

		if !errors.Is(err, domain.ErrQueueEmpty) {
			s.cb.RecordFailure()
			slog.Warn("redis pop failed, falling back to buffer", "error", err)
		}

		if errors.Is(err, domain.ErrQueueEmpty) {
			s.cb.RecordSuccess()
			if s.trySyncRefill(ctx) {
				popCtx2, cancel2 := context.WithTimeout(ctx, 100*time.Millisecond)
				key2, err2 := s.queue.Pop(popCtx2)
				cancel2()
				if err2 == nil {
					return key2, nil
				}
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.buffer) == 0 {
		if err := s.refillBuffer(ctx); err != nil {
			return "", fmt.Errorf("refill buffer: %w", err)
		}
		if len(s.buffer) == 0 {
			return "", domain.ErrPoolExhausted
		}
	}

	key := s.buffer[len(s.buffer)-1]
	s.buffer = s.buffer[:len(s.buffer)-1]

	if len(s.buffer) <= s.refillAt {
		go func() {
			refillCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.mu.Lock()
			defer s.mu.Unlock()
			if err := s.refillBuffer(refillCtx); err != nil {
				slog.Error("async buffer refill failed", "error", err)
			}
		}()
	}

	return key, nil
}

func (s *KeyGenService) maybeReactiveRefill() {
	if s.refillLock == nil {
		return
	}
	if !s.refilling.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.refilling.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.tryRefillQueue(ctx)
	}()
}

func (s *KeyGenService) trySyncRefill(ctx context.Context) bool {
	if s.refillLock == nil {
		return false
	}
	refillCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.tryRefillQueue(refillCtx)
}

func (s *KeyGenService) tryRefillQueue(ctx context.Context) bool {
	acquired, err := s.refillLock.AcquireIfBelow(ctx, s.queueLowWater)
	if err != nil {
		slog.Warn("refill lock check failed", "error", err)
		return false
	}
	if !acquired {
		return false
	}

	needed := int(s.queueHighWater)
	codes, err := s.repo.ClaimBatch(ctx, s.instanceID, needed)
	if err != nil {
		slog.Error("claim batch for queue refill failed", "error", err)
		return false
	}
	if len(codes) == 0 {
		return false
	}

	if err := s.queue.PushBatch(ctx, codes); err != nil {
		slog.Error("push batch to queue failed", "error", err)
		return false
	}

	slog.Info("queue refilled", "pushed", len(codes))
	return true
}

func (s *KeyGenService) fillQueue(ctx context.Context) error {
	qLen, err := s.queue.Len(ctx)
	if err != nil {
		return fmt.Errorf("queue len: %w", err)
	}
	if qLen >= s.queueLowWater {
		return nil
	}

	needed := int(s.queueHighWater - qLen)
	codes, err := s.repo.ClaimBatch(ctx, s.instanceID, needed)
	if err != nil {
		return fmt.Errorf("claim batch: %w", err)
	}
	if len(codes) == 0 {
		return nil
	}

	if err := s.queue.PushBatch(ctx, codes); err != nil {
		return fmt.Errorf("push batch: %w", err)
	}

	slog.Info("initial queue fill", "pushed", len(codes))
	return nil
}

func (s *KeyGenService) refillBuffer(ctx context.Context) error {
	needed := s.bufferSize - len(s.buffer)
	if needed <= 0 {
		return nil
	}

	codes, err := s.repo.ClaimBatch(ctx, s.instanceID, needed)
	if err != nil {
		return err
	}

	s.buffer = append(s.buffer, codes...)
	slog.Info("buffer refilled", "claimed", len(codes), "buffer_size", len(s.buffer))
	return nil
}

func (s *KeyGenService) queueRefillLoop(ctx context.Context) {
	ticker := time.NewTicker(s.queueRefillTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.cb.Allow() {
				continue
			}
			s.tryRefillQueue(ctx)
		}
	}
}

func (s *KeyGenService) poolReplenishLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := s.repo.CountAvailable(ctx)
			if err != nil {
				slog.Error("count available keys failed", "error", err)
				continue
			}

			if count < s.poolThreshold {
				slog.Info("pool below threshold, generating keys",
					"available", count,
					"threshold", s.poolThreshold,
					"generating", s.poolBatchSize,
				)
				inserted, err := s.repo.GenerateBatch(ctx, s.poolBatchSize)
				if err != nil {
					slog.Error("generate batch failed", "error", err)
					continue
				}
				slog.Info("keys generated", "inserted", inserted)
			}
		}
	}
}

func (s *KeyGenService) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(s.reconcileTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopReconcile:
			return
		case <-ticker.C:
			reclaimed, err := s.repo.ReclaimOrphaned(ctx, s.reconcileMaxAge)
			if err != nil {
				slog.Error("reconcile orphaned keys failed", "error", err)
				continue
			}
			if reclaimed > 0 {
				slog.Info("reclaimed orphaned keys", "count", reclaimed)
			}
		}
	}
}
