package service

import (
	"context"
	"fmt"
	"log/slog"

	pb "gotiny/proto/keygen"
	"gotiny/url-service/internal/domain"
	"gotiny/url-service/internal/port"
)

type URLService struct {
	repo         port.URLRepository
	keygenClient pb.KeyGenServiceClient
}

func NewURLService(repo port.URLRepository, keygenClient pb.KeyGenServiceClient) *URLService {
	return &URLService{
		repo:         repo,
		keygenClient: keygenClient,
	}
}

func (s *URLService) CreateShortURL(ctx context.Context, originalURL string, userID int64) (string, error) {
	resp, err := s.keygenClient.GetKey(ctx, &pb.GetKeyRequest{})
	if err != nil {
		return "", fmt.Errorf("get key from key-gen service: %w", err)
	}

	if err := s.repo.Store(ctx, resp.Key, originalURL, userID); err != nil {
		return "", fmt.Errorf("store url mapping: %w", err)
	}

	slog.Info("short url created", "short_code", resp.Key, "original_url", originalURL, "user_id", userID)
	return resp.Key, nil
}

func (s *URLService) GetURL(ctx context.Context, shortCode string, userID int64) (*domain.URL, error) {
	return s.repo.GetByShortCode(ctx, shortCode, userID)
}

func (s *URLService) ListURLs(ctx context.Context, userID int64, limit, offset int32) ([]*domain.URL, int32, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByUserID(ctx, userID, limit, offset)
}

func (s *URLService) DeleteURL(ctx context.Context, shortCode string, userID int64) error {
	return s.repo.DeleteByShortCode(ctx, shortCode, userID)
}
