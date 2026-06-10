package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var ErrPoolExhausted = errors.New("key pool exhausted")

type KeyRepository interface {
	ClaimBatch(ctx context.Context, instanceID string, batchSize int) ([]string, error)
	CountAvailable(ctx context.Context) (int64, error)
	GenerateBatch(ctx context.Context, count int) (int64, error)
}

type KeyGenService struct {
	repo       KeyRepository
	instanceID string

	mu     sync.Mutex
	buffer []string

	bufferSize    int
	refillAt      int
	poolThreshold int64
	poolBatchSize int
}

type Config struct {
	InstanceID    string
	BufferSize    int
	RefillAt      int
	PoolThreshold int64
	PoolBatchSize int
}

func DefaultConfig(instanceID string) Config {
	return Config{
		InstanceID:    instanceID,
		BufferSize:    100,
		RefillAt:      20,
		PoolThreshold: 10000,
		PoolBatchSize: 50000,
	}
}

func NewKeyGenService(repo KeyRepository, cfg Config) *KeyGenService {
	return &KeyGenService{
		repo:          repo,
		instanceID:    cfg.InstanceID,
		bufferSize:    cfg.BufferSize,
		refillAt:      cfg.RefillAt,
		poolThreshold: cfg.PoolThreshold,
		poolBatchSize: cfg.PoolBatchSize,
	}
}

// Init fills the buffer and starts the background pool replenisher.
func (s *KeyGenService) Init(ctx context.Context) error {
	if err := s.refillBuffer(ctx); err != nil {
		return fmt.Errorf("initial buffer fill: %w", err)
	}

	go s.replenishLoop(ctx)

	slog.Info("key-gen service initialized",
		"instance_id", s.instanceID,
		"buffer_size", len(s.buffer),
	)
	return nil
}

// GetKey returns the next available key from the buffer, refilling if needed.
func (s *KeyGenService) GetKey(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.buffer) == 0 {
		if err := s.refillBuffer(ctx); err != nil {
			return "", fmt.Errorf("refill buffer: %w", err)
		}
		if len(s.buffer) == 0 {
			return "", ErrPoolExhausted
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

// replenishLoop monitors the available key pool and generates new keys when low.
func (s *KeyGenService) replenishLoop(ctx context.Context) {
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
