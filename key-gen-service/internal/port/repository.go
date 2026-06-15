package port

import (
	"context"
	"time"
)

type KeyRepository interface {
	ClaimBatch(ctx context.Context, instanceID string, batchSize int) ([]string, error)
	CountAvailable(ctx context.Context) (int64, error)
	GenerateBatch(ctx context.Context, count int) (int64, error)
	ReclaimOrphaned(ctx context.Context, olderThan time.Duration) (int64, error)
}
