package port

import "context"

type URLReader interface {
	Resolve(ctx context.Context, shortCode string) (string, error)
}

type URLCache interface {
	Get(ctx context.Context, shortCode string) (string, error)
	Set(ctx context.Context, shortCode, originalURL string) error
	Delete(ctx context.Context, shortCode string) error
}
