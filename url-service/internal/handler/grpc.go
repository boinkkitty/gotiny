package handler

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/url"
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

	shortCode, err := h.svc.CreateShortURL(ctx, req.OriginalUrl)
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
