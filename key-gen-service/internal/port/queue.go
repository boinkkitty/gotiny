package port

import "context"

type KeyQueue interface {
	Pop(ctx context.Context) (string, error)
	PushBatch(ctx context.Context, keys []string) error
	Len(ctx context.Context) (int64, error)
}

type RefillLock interface {
	AcquireIfBelow(ctx context.Context, threshold int64) (bool, error)
}
