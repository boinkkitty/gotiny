package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type URLRepository struct {
	pool *pgxpool.Pool
}

func NewURLRepository(pool *pgxpool.Pool) *URLRepository {
	return &URLRepository{pool: pool}
}

func (r *URLRepository) Store(ctx context.Context, shortCode, originalURL string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO urls (short_code, original_url) VALUES ($1, $2)`,
		shortCode, originalURL,
	)
	if err != nil {
		return fmt.Errorf("insert url mapping: %w", err)
	}
	return nil
}
