package service

import (
	"context"
	"errors"
	"testing"

	"gotiny/redirect-service/internal/domain"
)

type mockReader struct {
	url string
	err error
}

func (m *mockReader) Resolve(_ context.Context, _ string) (string, error) {
	return m.url, m.err
}

type mockCache struct {
	store    map[string]string
	getErr   error
	setErr   error
	delErr   error
	getCalls int
	setCalls int
	delCalls int
}

func newMockCache() *mockCache {
	return &mockCache{store: make(map[string]string)}
}

func (m *mockCache) Get(_ context.Context, shortCode string) (string, error) {
	m.getCalls++
	if m.getErr != nil {
		return "", m.getErr
	}
	if v, ok := m.store[shortCode]; ok {
		return v, nil
	}
	return "", errors.New("not in cache")
}

func (m *mockCache) Set(_ context.Context, shortCode, url string) error {
	m.setCalls++
	if m.setErr != nil {
		return m.setErr
	}
	m.store[shortCode] = url
	return nil
}

func (m *mockCache) Delete(_ context.Context, shortCode string) error {
	m.delCalls++
	if m.delErr != nil {
		return m.delErr
	}
	delete(m.store, shortCode)
	return nil
}

func TestResolve_CacheHit(t *testing.T) {
	cache := newMockCache()
	cache.store["abc"] = "https://example.com"
	reader := &mockReader{url: "https://db.com"}

	svc := NewRedirectService(reader, cache)
	url, err := svc.Resolve(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://example.com" {
		t.Errorf("expected cache value, got %q", url)
	}
}

func TestResolve_CacheMiss_FallsBackToDB(t *testing.T) {
	cache := newMockCache()
	reader := &mockReader{url: "https://db.com"}

	svc := NewRedirectService(reader, cache)
	url, err := svc.Resolve(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://db.com" {
		t.Errorf("expected db value, got %q", url)
	}
	if cache.setCalls != 1 {
		t.Errorf("expected 1 cache set call, got %d", cache.setCalls)
	}
}

func TestResolve_NilCache_UsesDBDirectly(t *testing.T) {
	reader := &mockReader{url: "https://db.com"}

	svc := NewRedirectService(reader, nil)
	url, err := svc.Resolve(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://db.com" {
		t.Errorf("expected db value, got %q", url)
	}
}

func TestResolve_NotFound(t *testing.T) {
	cache := newMockCache()
	reader := &mockReader{err: domain.ErrNotFound}

	svc := NewRedirectService(reader, cache)
	_, err := svc.Resolve(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestResolve_CacheSetFailure_StillReturns(t *testing.T) {
	cache := newMockCache()
	cache.setErr = errors.New("redis down")
	reader := &mockReader{url: "https://db.com"}

	svc := NewRedirectService(reader, cache)
	url, err := svc.Resolve(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://db.com" {
		t.Errorf("expected db value despite cache set failure, got %q", url)
	}
}

func TestInvalidateCache_Success(t *testing.T) {
	cache := newMockCache()
	cache.store["abc"] = "https://example.com"

	svc := NewRedirectService(&mockReader{}, cache)
	if err := svc.InvalidateCache(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cache.delCalls != 1 {
		t.Errorf("expected 1 delete call, got %d", cache.delCalls)
	}
}

func TestInvalidateCache_NilCache(t *testing.T) {
	svc := NewRedirectService(&mockReader{}, nil)
	if err := svc.InvalidateCache(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
