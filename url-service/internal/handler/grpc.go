package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gotiny/pkg/grpcutil"
	pb "gotiny/proto/url"
	"gotiny/url-service/internal/repository"
	"gotiny/url-service/internal/service"
)

type URLHandler struct {
	pb.UnimplementedURLServiceServer
	svc *service.URLService
}

func NewURLHandler(svc *service.URLService) *URLHandler {
	return &URLHandler{svc: svc}
}

func (h *URLHandler) CreateShortURL(ctx context.Context, req *pb.CreateShortURLRequest) (*pb.CreateShortURLResponse, error) {
	if req.OriginalUrl == "" {
		return nil, status.Error(codes.InvalidArgument, "original_url is required")
	}

	userID, ok := grpcutil.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "user_id is required")
	}

	shortCode, err := h.svc.CreateShortURL(ctx, req.OriginalUrl, userID)
	if err != nil {
		slog.Error("create short url failed", "error", err)
		st, ok := status.FromError(err)
		if ok {
			return nil, st.Err()
		}
		return nil, status.Error(codes.Internal, "failed to create short url")
	}

	return &pb.CreateShortURLResponse{ShortCode: shortCode}, nil
}

func (h *URLHandler) GetURL(ctx context.Context, req *pb.GetURLRequest) (*pb.GetURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	u, err := h.svc.GetURL(ctx, req.ShortCode, req.UserId)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "url %q not found", req.ShortCode)
		}
		slog.Error("get url failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to get url")
	}

	return &pb.GetURLResponse{
		ShortCode:   u.ShortCode,
		OriginalUrl: u.OriginalURL,
		CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (h *URLHandler) ListURLs(ctx context.Context, req *pb.ListURLsRequest) (*pb.ListURLsResponse, error) {
	urls, total, err := h.svc.ListURLs(ctx, req.UserId, req.Limit, req.Offset)
	if err != nil {
		slog.Error("list urls failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to list urls")
	}

	resp := &pb.ListURLsResponse{Total: total}
	for _, u := range urls {
		resp.Urls = append(resp.Urls, &pb.GetURLResponse{
			ShortCode:   u.ShortCode,
			OriginalUrl: u.OriginalURL,
			CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return resp, nil
}

func (h *URLHandler) DeleteURL(ctx context.Context, req *pb.DeleteURLRequest) (*pb.DeleteURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	err := h.svc.DeleteURL(ctx, req.ShortCode, req.UserId)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "url %q not found", req.ShortCode)
		}
		if errors.Is(err, repository.ErrNotOwner) {
			return nil, status.Error(codes.PermissionDenied, "you do not own this url")
		}
		slog.Error("delete url failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete url")
	}

	return &pb.DeleteURLResponse{}, nil
}
