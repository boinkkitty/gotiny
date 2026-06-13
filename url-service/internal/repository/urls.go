package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("url not found")
	ErrNotOwner      = errors.New("not the owner of this url")
)

type URL struct {
	ShortCode   string
	OriginalURL string
	CreatedAt   time.Time
}

type URLRepository struct {
	pool *pgxpool.Pool
}

func NewURLRepository(pool *pgxpool.Pool) *URLRepository {
	return &URLRepository{pool: pool}
}

func (r *URLRepository) Store(ctx context.Context, shortCode, originalURL string, userID int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO urls (short_code, original_url, user_id) VALUES ($1, $2, $3)`,
		shortCode, originalURL, userID,
	)
	if err != nil {
		return fmt.Errorf("insert url mapping: %w", err)
	}
	return nil
}

func (r *URLRepository) GetByShortCode(ctx context.Context, shortCode string, userID int64) (*URL, error) {
	var u URL
	err := r.pool.QueryRow(ctx,
		`SELECT short_code, original_url, created_at FROM urls WHERE short_code = $1 AND user_id = $2`,
		shortCode, userID,
	).Scan(&u.ShortCode, &u.OriginalURL, &u.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query url: %w", err)
	}
	return &u, nil
}

func (r *URLRepository) ListByUserID(ctx context.Context, userID int64, limit, offset int32) ([]*URL, int32, error) {
	var total int32
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM urls WHERE user_id = $1`,
		userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count urls: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT short_code, original_url, created_at FROM urls WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list urls: %w", err)
	}
	defer rows.Close()

	var urls []*URL
	for rows.Next() {
		var u URL
		if err := rows.Scan(&u.ShortCode, &u.OriginalURL, &u.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan url: %w", err)
		}
		urls = append(urls, &u)
	}
	return urls, total, nil
}

func (r *URLRepository) DeleteByShortCode(ctx context.Context, shortCode string, userID int64) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM urls WHERE short_code = $1 AND user_id = $2`,
		shortCode, userID,
	)
	if err != nil {
		return fmt.Errorf("delete url: %w", err)
	}

	if tag.RowsAffected() == 0 {
		var exists bool
		err := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)`,
			shortCode,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check url existence: %w", err)
		}
		if exists {
			return ErrNotOwner
		}
		return ErrNotFound
	}
	return nil
}
