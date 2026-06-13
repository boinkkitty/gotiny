package domain

import "time"

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type RefreshToken struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
}
