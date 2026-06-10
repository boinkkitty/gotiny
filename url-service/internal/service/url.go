package service

import (
	"context"
	"fmt"
	"log/slog"

	pb "gotiny/proto/keygen"
	"gotiny/url-service/internal/repository"
)

type URLService struct {
	repo         *repository.URLRepository
	keygenClient pb.KeyGenServiceClient
}

func NewURLService(repo *repository.URLRepository, keygenClient pb.KeyGenServiceClient) *URLService {
	return &URLService{
		repo:         repo,
		keygenClient: keygenClient,
	}
}

func (s *URLService) CreateShortURL(ctx context.Context, originalURL string) (string, error) {
	resp, err := s.keygenClient.GetKey(ctx, &pb.GetKeyRequest{})
	if err != nil {
		return "", fmt.Errorf("get key from key-gen service: %w", err)
	}

	if err := s.repo.Store(ctx, resp.Key, originalURL); err != nil {
		return "", fmt.Errorf("store url mapping: %w", err)
	}

	slog.Info("short url created", "short_code", resp.Key, "original_url", originalURL)
	return resp.Key, nil
}
