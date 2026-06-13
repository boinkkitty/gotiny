package port

import (
	"context"

	"gotiny/url-service/internal/domain"
)

type URLRepository interface {
	Store(ctx context.Context, shortCode, originalURL string, userID int64) error
	GetByShortCode(ctx context.Context, shortCode string, userID int64) (*domain.URL, error)
	ListByUserID(ctx context.Context, userID int64, limit, offset int32) ([]*domain.URL, int32, error)
	DeleteByShortCode(ctx context.Context, shortCode string, userID int64) error
}
