package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gotiny/redirect-service/internal/domain"
)

type URLRepository struct {
	pool *pgxpool.Pool
}

func NewURLRepository(pool *pgxpool.Pool) *URLRepository {
	return &URLRepository{pool: pool}
}

func (r *URLRepository) Resolve(ctx context.Context, shortCode string) (string, error) {
	var originalURL string
	err := r.pool.QueryRow(ctx,
		`SELECT original_url FROM urls WHERE short_code = $1`, shortCode,
	).Scan(&originalURL)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query url: %w", err)
	}
	return originalURL, nil
}
