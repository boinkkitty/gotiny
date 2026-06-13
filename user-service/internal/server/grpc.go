package server

import (
	"context"
	"errors"
	"log/slog"
	"net/mail"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/user"
	"gotiny/user-service/internal/domain"
	"gotiny/user-service/internal/service"
)

const minPasswordLength = 8

type UserHandler struct {
	pb.UnimplementedUserServiceServer
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	if _, err := mail.ParseAddress(req.Email); err != nil || req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "valid email is required")
	}
	if len(req.Password) < minPasswordLength {
		return nil, status.Errorf(codes.InvalidArgument, "password must be at least %d characters", minPasswordLength)
	}

	result, err := h.svc.Register(ctx, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrEmailExists) {
			return nil, status.Error(codes.AlreadyExists, "email already registered")
		}
		slog.Error("register failed", "error", err)
		return nil, status.Error(codes.Internal, "registration failed")
	}

	return toAuthResponse(result), nil
}

func (h *UserHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	result, err := h.svc.Login(ctx, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		}
		slog.Error("login failed", "error", err)
		return nil, status.Error(codes.Internal, "login failed")
	}

	return toAuthResponse(result), nil
}

func (h *UserHandler) RefreshToken(ctx context.Context, req *pb.RefreshRequest) (*pb.AuthResponse, error) {
	if req.RefreshToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	result, err := h.svc.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrTokenInvalid) {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired refresh token")
		}
		slog.Error("refresh failed", "error", err)
		return nil, status.Error(codes.Internal, "token refresh failed")
	}

	return toAuthResponse(result), nil
}

func (h *UserHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	if req.RefreshToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	if err := h.svc.Logout(ctx, req.RefreshToken); err != nil {
		if errors.Is(err, domain.ErrTokenInvalid) {
			return &pb.LogoutResponse{}, nil
		}
		slog.Error("logout failed", "error", err)
		return nil, status.Error(codes.Internal, "logout failed")
	}

	return &pb.LogoutResponse{}, nil
}

func toAuthResponse(r *service.AuthResult) *pb.AuthResponse {
	return &pb.AuthResponse{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		ExpiresIn:    r.ExpiresIn,
	}
}
