package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"gotiny/user-service/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) CreateUser(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, password_hash, created_at`,
		email, passwordHash,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrEmailExists
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, created_at FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user by email: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) StoreRefreshToken(ctx context.Context, userID int64, expiresAt time.Time) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	_, err = r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, token, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert refresh token: %w", err)
	}
	return token, nil
}

func (r *UserRepository) GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	var rt domain.RefreshToken
	var revoked bool
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token, expires_at, revoked FROM refresh_tokens WHERE token = $1`,
		token,
	).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &revoked)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("query refresh token: %w", err)
	}
	if revoked || time.Now().After(rt.ExpiresAt) {
		return nil, domain.ErrTokenInvalid
	}
	return &rt, nil
}

func (r *UserRepository) RevokeRefreshToken(ctx context.Context, token string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked = TRUE WHERE token = $1 AND revoked = FALSE`,
		token,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTokenInvalid
	}
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
