package port

import (
	"context"
	"time"

	"gotiny/user-service/internal/domain"
)

type UserRepository interface {
	CreateUser(ctx context.Context, email, passwordHash string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	StoreRefreshToken(ctx context.Context, userID int64, expiresAt time.Time) (string, error)
	GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, token string) error
}
