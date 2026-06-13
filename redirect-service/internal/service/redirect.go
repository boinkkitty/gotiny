package service

import (
	"context"
	"errors"
	"log/slog"

	"gotiny/redirect-service/internal/domain"
	"gotiny/redirect-service/internal/port"
)

type RedirectService struct {
	reader port.URLReader
	cache  port.URLCache
}

func NewRedirectService(reader port.URLReader, cache port.URLCache) *RedirectService {
	return &RedirectService{reader: reader, cache: cache}
}

func (s *RedirectService) Resolve(ctx context.Context, shortCode string) (string, error) {
	if s.cache != nil {
		cached, err := s.cache.Get(ctx, shortCode)
		if err == nil {
			slog.Debug("cache hit", "short_code", shortCode)
			return cached, nil
		}
		if !errors.Is(err, domain.ErrCacheMiss) {
			slog.Warn("cache get failed, falling back to postgres", "error", err, "short_code", shortCode)
		}
	}

	originalURL, err := s.reader.Resolve(ctx, shortCode)
	if err != nil {
		return "", err
	}

	if s.cache != nil {
		if err := s.cache.Set(ctx, shortCode, originalURL); err != nil {
			slog.Warn("cache set failed", "error", err)
		}
	}

	return originalURL, nil
}

func (s *RedirectService) InvalidateCache(ctx context.Context, shortCode string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, shortCode)
}
