package repository

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgxpool"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

type KeysRepository struct {
	pool *pgxpool.Pool
}

func NewKeysRepository(pool *pgxpool.Pool) *KeysRepository {
	return &KeysRepository{pool: pool}
}

// ClaimBatch atomically claims a batch of available keys for the given instance.
func (r *KeysRepository) ClaimBatch(ctx context.Context, instanceID string, batchSize int) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT id, code FROM keys WHERE status = 'available' LIMIT $1 FOR UPDATE SKIP LOCKED`,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("select available keys: %w", err)
	}

	var ids []int64
	var codes []string
	for rows.Next() {
		var id int64
		var code string
		if err := rows.Scan(&id, &code); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan key row: %w", err)
		}
		ids = append(ids, id)
		codes = append(codes, code)
	}
	rows.Close()

	if len(ids) == 0 {
		return nil, nil
	}

	_, err = tx.Exec(ctx,
		`UPDATE keys SET status = 'claimed', claimed_by = $1, claimed_at = NOW() WHERE id = ANY($2)`,
		instanceID, ids,
	)
	if err != nil {
		return nil, fmt.Errorf("update keys to claimed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return codes, nil
}

// MarkUsed transitions a key from claimed to used.
func (r *KeysRepository) MarkUsed(ctx context.Context, code string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE keys SET status = 'used', used_at = NOW() WHERE code = $1 AND status = 'claimed'`,
		code,
	)
	return err
}

// CountAvailable returns the number of keys with status 'available'.
func (r *KeysRepository) CountAvailable(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM keys WHERE status = 'available'`).Scan(&count)
	return count, err
}

// GenerateBatch inserts a batch of random base62 keys into the pool.
func (r *KeysRepository) GenerateBatch(ctx context.Context, count int) (int64, error) {
	const keyLength = 7

	query := `INSERT INTO keys (code) VALUES ($1) ON CONFLICT (code) DO NOTHING`
	var inserted int64

	for i := 0; i < count; i++ {
		code, err := randomBase62(keyLength)
		if err != nil {
			return inserted, fmt.Errorf("generate random key: %w", err)
		}
		tag, err := r.pool.Exec(ctx, query, code)
		if err != nil {
			return inserted, fmt.Errorf("insert key: %w", err)
		}
		inserted += tag.RowsAffected()
	}

	return inserted, nil
}

func randomBase62(length int) (string, error) {
	max := big.NewInt(int64(len(base62Chars)))
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		result[i] = base62Chars[n.Int64()]
	}
	return string(result), nil
}
