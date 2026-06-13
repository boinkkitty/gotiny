package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/redirect"
	"gotiny/redirect-service/internal/repository"
)

type URLResolver interface {
	Resolve(ctx context.Context, shortCode string) (string, error)
	InvalidateCache(ctx context.Context, shortCode string) error
}

type RedirectHandler struct {
	pb.UnimplementedRedirectServiceServer
	repo URLResolver
}

func NewRedirectHandler(repo URLResolver) *RedirectHandler {
	return &RedirectHandler{repo: repo}
}

func (h *RedirectHandler) Resolve(ctx context.Context, req *pb.ResolveRequest) (*pb.ResolveResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	originalURL, err := h.repo.Resolve(ctx, req.ShortCode)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "short code %q not found", req.ShortCode)
		}
		slog.Error("resolve failed", "error", err, "short_code", req.ShortCode)
		return nil, status.Error(codes.Unavailable, "service unavailable")
	}

	return &pb.ResolveResponse{OriginalUrl: originalURL}, nil
}

func (h *RedirectHandler) InvalidateCache(ctx context.Context, req *pb.InvalidateCacheRequest) (*pb.InvalidateCacheResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	if err := h.repo.InvalidateCache(ctx, req.ShortCode); err != nil {
		slog.Error("invalidate cache failed", "error", err, "short_code", req.ShortCode)
		return nil, status.Error(codes.Internal, "cache invalidation failed")
	}

	return &pb.InvalidateCacheResponse{}, nil
}
