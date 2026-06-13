package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gotiny/user-service/internal/repository"
)

type mockUserRepo struct {
	createUser    *repository.User
	createErr     error
	getUser       *repository.User
	getErr        error
	storeToken    string
	storeTokenErr error
	getToken      *repository.RefreshToken
	getTokenErr   error
	revokeErr     error
}

func (m *mockUserRepo) CreateUser(_ context.Context, email, passwordHash string) (*repository.User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.createUser != nil {
		return m.createUser, nil
	}
	return &repository.User{ID: 1, Email: email, PasswordHash: passwordHash}, nil
}

func (m *mockUserRepo) GetUserByEmail(_ context.Context, _ string) (*repository.User, error) {
	return m.getUser, m.getErr
}

func (m *mockUserRepo) StoreRefreshToken(_ context.Context, _ int64, _ time.Time) (string, error) {
	if m.storeTokenErr != nil {
		return "", m.storeTokenErr
	}
	if m.storeToken != "" {
		return m.storeToken, nil
	}
	return "refresh-token-abc", nil
}

func (m *mockUserRepo) GetRefreshToken(_ context.Context, _ string) (*repository.RefreshToken, error) {
	return m.getToken, m.getTokenErr
}

func (m *mockUserRepo) RevokeRefreshToken(_ context.Context, _ string) error {
	return m.revokeErr
}

func TestRegister_Success(t *testing.T) {
	svc := NewUserService(&mockUserRepo{}, "test-secret")

	result, err := svc.Register(context.Background(), "user@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if result.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if result.ExpiresIn != 900 {
		t.Errorf("expected expires_in=900, got %d", result.ExpiresIn)
	}
}

func TestRegister_EmailExists(t *testing.T) {
	svc := NewUserService(
		&mockUserRepo{createErr: repository.ErrEmailExists},
		"test-secret",
	)

	_, err := svc.Register(context.Background(), "existing@example.com", "password123")
	if !errors.Is(err, repository.ErrEmailExists) {
		t.Fatalf("expected ErrEmailExists, got %v", err)
	}
}

func TestLogin_Success(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcryptCost)
	svc := NewUserService(
		&mockUserRepo{
			getUser: &repository.User{ID: 1, Email: "user@example.com", PasswordHash: string(hash)},
		},
		"test-secret",
	)

	result, err := svc.Login(context.Background(), "user@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcryptCost)
	svc := NewUserService(
		&mockUserRepo{
			getUser: &repository.User{ID: 1, Email: "user@example.com", PasswordHash: string(hash)},
		},
		"test-secret",
	)

	_, err := svc.Login(context.Background(), "user@example.com", "wrong-password")
	if !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound (no user enumeration), got %v", err)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	svc := NewUserService(
		&mockUserRepo{getErr: repository.ErrUserNotFound},
		"test-secret",
	)

	_, err := svc.Login(context.Background(), "nobody@example.com", "password123")
	if !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestRefreshToken_Success(t *testing.T) {
	svc := NewUserService(
		&mockUserRepo{
			getToken: &repository.RefreshToken{
				ID:        1,
				UserID:    1,
				Token:     "old-token",
				ExpiresAt: time.Now().Add(time.Hour),
			},
		},
		"test-secret",
	)

	result, err := svc.RefreshToken(context.Background(), "old-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

func TestRefreshToken_Invalid(t *testing.T) {
	svc := NewUserService(
		&mockUserRepo{getTokenErr: repository.ErrTokenInvalid},
		"test-secret",
	)

	_, err := svc.RefreshToken(context.Background(), "bad-token")
	if !errors.Is(err, repository.ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestLogout_Success(t *testing.T) {
	svc := NewUserService(&mockUserRepo{}, "test-secret")

	err := svc.Logout(context.Background(), "some-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
