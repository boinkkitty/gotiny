package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/keygen"

	"gotiny/key-gen-service/internal/service"
)

type KeyGenHandler struct {
	pb.UnimplementedKeyGenServiceServer
	svc *service.KeyGenService
}

func NewKeyGenHandler(svc *service.KeyGenService) *KeyGenHandler {
	return &KeyGenHandler{svc: svc}
}

func (h *KeyGenHandler) GetKey(ctx context.Context, _ *pb.GetKeyRequest) (*pb.GetKeyResponse, error) {
	key, err := h.svc.GetKey(ctx)
	if err != nil {
		slog.Error("get key failed", "error", err)
		if errors.Is(err, service.ErrPoolExhausted) {
			return nil, status.Error(codes.ResourceExhausted, "key pool exhausted")
		}
		return nil, status.Error(codes.Unavailable, "key generation unavailable")
	}

	return &pb.GetKeyResponse{Key: key}, nil
}
