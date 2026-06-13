package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"gotiny/user-service/internal/domain"
	"gotiny/user-service/internal/port"
)

const bcryptCost = 10

type UserService struct {
	repo            port.UserRepository
	jwtSecret       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewUserService(repo port.UserRepository, jwtSecret string) *UserService {
	return &UserService{
		repo:            repo,
		jwtSecret:       []byte(jwtSecret),
		accessTokenTTL:  15 * time.Minute,
		refreshTokenTTL: 7 * 24 * time.Hour,
	}
}

type AuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

func (s *UserService) Register(ctx context.Context, email, password string) (*AuthResult, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, email, string(hash))
	if err != nil {
		return nil, err
	}

	slog.Info("user registered", "user_id", user.ID, "email", user.Email)
	return s.issueTokens(ctx, user.ID)
}

func (s *UserService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrUserNotFound
	}

	slog.Info("user logged in", "user_id", user.ID)
	return s.issueTokens(ctx, user.ID)
}

func (s *UserService) RefreshToken(ctx context.Context, refreshToken string) (*AuthResult, error) {
	rt, err := s.repo.GetRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	if err := s.repo.RevokeRefreshToken(ctx, refreshToken); err != nil {
		return nil, fmt.Errorf("revoke old token: %w", err)
	}

	return s.issueTokens(ctx, rt.UserID)
}

func (s *UserService) Logout(ctx context.Context, refreshToken string) error {
	return s.repo.RevokeRefreshToken(ctx, refreshToken)
}

func (s *UserService) issueTokens(ctx context.Context, userID int64) (*AuthResult, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(s.accessTokenTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshToken, err := s.repo.StoreRefreshToken(ctx, userID, now.Add(s.refreshTokenTTL))
	if err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}
