package domain

import "errors"

var (
	ErrEmailExists  = errors.New("email already registered")
	ErrUserNotFound = errors.New("user not found")
	ErrTokenInvalid = errors.New("refresh token invalid or expired")
)
