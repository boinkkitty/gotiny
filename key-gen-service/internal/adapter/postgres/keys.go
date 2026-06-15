package postgres

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

type KeysRepository struct {
	pool *pgxpool.Pool
}

func NewKeysRepository(pool *pgxpool.Pool) *KeysRepository {
	return &KeysRepository{pool: pool}
}

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate key rows: %w", err)
	}

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

func (r *KeysRepository) CountAvailable(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM keys WHERE status = 'available'`).Scan(&count)
	return count, err
}

func (r *KeysRepository) GenerateBatch(ctx context.Context, count int) (int64, error) {
	const keyLength = 7
	const batchSize = 1000

	var totalInserted int64

	for start := 0; start < count; start += batchSize {
		end := start + batchSize
		if end > count {
			end = count
		}
		n := end - start

		codes := make([]string, 0, n)
		for i := 0; i < n; i++ {
			code, err := randomBase62(keyLength)
			if err != nil {
				return totalInserted, fmt.Errorf("generate random key: %w", err)
			}
			codes = append(codes, code)
		}

		query := `INSERT INTO keys (code) SELECT unnest($1::varchar[]) ON CONFLICT (code) DO NOTHING`
		tag, err := r.pool.Exec(ctx, query, codes)
		if err != nil {
			return totalInserted, fmt.Errorf("batch insert keys: %w", err)
		}
		totalInserted += tag.RowsAffected()
	}

	return totalInserted, nil
}

func (r *KeysRepository) ReclaimOrphaned(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE keys SET status = 'available', claimed_by = NULL, claimed_at = NULL
		 WHERE status = 'claimed' AND claimed_at < NOW() - $1::interval AND used_at IS NULL`,
		olderThan.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("reclaim orphaned keys: %w", err)
	}
	return tag.RowsAffected(), nil
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
