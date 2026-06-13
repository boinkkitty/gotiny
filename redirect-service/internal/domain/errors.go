package domain

import "errors"

var (
	ErrNotFound  = errors.New("short code not found")
	ErrCacheMiss = errors.New("cache miss")
)
