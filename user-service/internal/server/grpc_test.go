package server_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/user"
	"gotiny/user-service/internal/domain"
	"gotiny/user-service/internal/server"
	"gotiny/user-service/internal/service"
)

type mockUserRepo struct {
	createUser  *domain.User
	createErr   error
	getUser     *domain.User
	getErr      error
	getToken    *domain.RefreshToken
	getTokenErr error
	revokeErr   error
}

func (m *mockUserRepo) CreateUser(_ context.Context, email, hash string) (*domain.User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.createUser != nil {
		return m.createUser, nil
	}
	return &domain.User{ID: 1, Email: email, PasswordHash: hash}, nil
}
func (m *mockUserRepo) GetUserByEmail(_ context.Context, _ string) (*domain.User, error) {
	return m.getUser, m.getErr
}
func (m *mockUserRepo) StoreRefreshToken(_ context.Context, _ int64, _ time.Time) (string, error) {
	return "refresh-token", nil
}
func (m *mockUserRepo) GetRefreshToken(_ context.Context, _ string) (*domain.RefreshToken, error) {
	return m.getToken, m.getTokenErr
}
func (m *mockUserRepo) RevokeRefreshToken(_ context.Context, _ string) error {
	return m.revokeErr
}

func TestRegister_InvalidEmail(t *testing.T) {
	svc := service.NewUserService(&mockUserRepo{}, "secret")
	h := server.NewUserHandler(svc)

	_, err := h.Register(context.Background(), &pb.RegisterRequest{
		Email: "not-an-email", Password: "password123",
	})
	if err == nil {
		t.Fatal("expected error for invalid email")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	svc := service.NewUserService(&mockUserRepo{}, "secret")
	h := server.NewUserHandler(svc)

	_, err := h.Register(context.Background(), &pb.RegisterRequest{
		Email: "user@example.com", Password: "short",
	})
	if err == nil {
		t.Fatal("expected error for short password")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestRegister_Success(t *testing.T) {
	svc := service.NewUserService(&mockUserRepo{}, "secret")
	h := server.NewUserHandler(svc)

	resp, err := h.Register(context.Background(), &pb.RegisterRequest{
		Email: "user@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

func TestRegister_EmailExists(t *testing.T) {
	svc := service.NewUserService(
		&mockUserRepo{createErr: domain.ErrEmailExists},
		"secret",
	)
	h := server.NewUserHandler(svc)

	_, err := h.Register(context.Background(), &pb.RegisterRequest{
		Email: "existing@example.com", Password: "password123",
	})
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", st.Code())
	}
}

func TestLogin_EmptyFields(t *testing.T) {
	svc := service.NewUserService(&mockUserRepo{}, "secret")
	h := server.NewUserHandler(svc)

	_, err := h.Login(context.Background(), &pb.LoginRequest{Email: "", Password: ""})
	if err == nil {
		t.Fatal("expected error for empty fields")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	svc := service.NewUserService(
		&mockUserRepo{getErr: domain.ErrUserNotFound},
		"secret",
	)
	h := server.NewUserHandler(svc)

	_, err := h.Login(context.Background(), &pb.LoginRequest{
		Email: "nobody@example.com", Password: "password123",
	})
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestRefresh_EmptyToken(t *testing.T) {
	svc := service.NewUserService(&mockUserRepo{}, "secret")
	h := server.NewUserHandler(svc)

	_, err := h.RefreshToken(context.Background(), &pb.RefreshRequest{RefreshToken: ""})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestLogout_EmptyToken(t *testing.T) {
	svc := service.NewUserService(&mockUserRepo{}, "secret")
	h := server.NewUserHandler(svc)

	_, err := h.Logout(context.Background(), &pb.LogoutRequest{RefreshToken: ""})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}
