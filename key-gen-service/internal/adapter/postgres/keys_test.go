package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gotiny/key-gen-service/internal/adapter/postgres"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := "postgres://postgres:postgres@localhost:5432/gotiny_test?sslmode=disable"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: %v", err)
	}

	_, err = pool.Exec(ctx, `
		DROP TABLE IF EXISTS keys;
		CREATE TABLE keys (
			id BIGSERIAL PRIMARY KEY,
			code VARCHAR(7) NOT NULL UNIQUE,
			status VARCHAR(10) NOT NULL DEFAULT 'available',
			claimed_by UUID,
			claimed_at TIMESTAMPTZ,
			used_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_keys_status ON keys (status) WHERE status = 'available';
	`)
	if err != nil {
		t.Fatalf("setup schema: %v", err)
	}

	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestGenerateBatch(t *testing.T) {
	pool := setupTestDB(t)
	repo := postgres.NewKeysRepository(pool)
	ctx := context.Background()

	inserted, err := repo.GenerateBatch(ctx, 100)
	if err != nil {
		t.Fatalf("generate batch: %v", err)
	}
	if inserted != 100 {
		t.Errorf("expected 100 inserted, got %d", inserted)
	}

	count, err := repo.CountAvailable(ctx)
	if err != nil {
		t.Fatalf("count available: %v", err)
	}
	if count != 100 {
		t.Errorf("expected 100 available, got %d", count)
	}
}

func TestClaimBatch(t *testing.T) {
	pool := setupTestDB(t)
	repo := postgres.NewKeysRepository(pool)
	ctx := context.Background()

	if _, err := repo.GenerateBatch(ctx, 50); err != nil {
		t.Fatalf("generate: %v", err)
	}

	codes, err := repo.ClaimBatch(ctx, "00000000-0000-0000-0000-000000000001", 20)
	if err != nil {
		t.Fatalf("claim batch: %v", err)
	}
	if len(codes) != 20 {
		t.Errorf("expected 20 codes, got %d", len(codes))
	}

	remaining, _ := repo.CountAvailable(ctx)
	if remaining != 30 {
		t.Errorf("expected 30 remaining, got %d", remaining)
	}
}

func TestClaimBatchExhausted(t *testing.T) {
	pool := setupTestDB(t)
	repo := postgres.NewKeysRepository(pool)
	ctx := context.Background()

	codes, err := repo.ClaimBatch(ctx, "00000000-0000-0000-0000-000000000001", 10)
	if err != nil {
		t.Fatalf("claim from empty pool: %v", err)
	}
	if len(codes) != 0 {
		t.Errorf("expected 0 codes from empty pool, got %d", len(codes))
	}
}

func TestReclaimOrphaned(t *testing.T) {
	pool := setupTestDB(t)
	repo := postgres.NewKeysRepository(pool)
	ctx := context.Background()

	if _, err := repo.GenerateBatch(ctx, 10); err != nil {
		t.Fatalf("generate: %v", err)
	}

	codes, err := repo.ClaimBatch(ctx, "00000000-0000-0000-0000-000000000001", 5)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(codes) != 5 {
		t.Fatalf("expected 5 claimed, got %d", len(codes))
	}

	// Backdate claimed_at so they qualify for reclamation
	_, err = pool.Exec(ctx,
		`UPDATE keys SET claimed_at = NOW() - INTERVAL '2 hours' WHERE status = 'claimed'`,
	)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	reclaimed, err := repo.ReclaimOrphaned(ctx, time.Hour)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if reclaimed != 5 {
		t.Errorf("expected 5 reclaimed, got %d", reclaimed)
	}

	available, _ := repo.CountAvailable(ctx)
	if available != 10 {
		t.Errorf("expected 10 available after reclaim, got %d", available)
	}
}

func TestMultiInstanceCoordination(t *testing.T) {
	pool := setupTestDB(t)
	repo := postgres.NewKeysRepository(pool)
	ctx := context.Background()

	if _, err := repo.GenerateBatch(ctx, 2000); err != nil {
		t.Fatalf("generate: %v", err)
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		codes1 []string
		codes2 []string
		err1   error
		err2   error
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			batch, err := repo.ClaimBatch(ctx, "00000000-0000-0000-0000-000000000001", 100)
			if err != nil {
				mu.Lock()
				err1 = err
				mu.Unlock()
				return
			}
			mu.Lock()
			codes1 = append(codes1, batch...)
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			batch, err := repo.ClaimBatch(ctx, "00000000-0000-0000-0000-000000000002", 100)
			if err != nil {
				mu.Lock()
				err2 = err
				mu.Unlock()
				return
			}
			mu.Lock()
			codes2 = append(codes2, batch...)
			mu.Unlock()
		}
	}()

	wg.Wait()

	if err1 != nil {
		t.Fatalf("instance-1 error: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("instance-2 error: %v", err2)
	}

	seen := make(map[string]string)
	for _, code := range codes1 {
		seen[code] = "inst-1"
	}
	for _, code := range codes2 {
		if owner, exists := seen[code]; exists {
			t.Errorf("DUPLICATE: code %q claimed by both %s and inst-2", code, owner)
		}
		seen[code] = "inst-2"
	}

	total := len(codes1) + len(codes2)
	t.Logf("instance-1: %d keys, instance-2: %d keys, total: %d, unique: %d",
		len(codes1), len(codes2), total, len(seen))

	if total != len(seen) {
		t.Errorf("duplicates detected: total=%d unique=%d", total, len(seen))
	}
}
