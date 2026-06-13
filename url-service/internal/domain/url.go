package domain

import "time"

type URL struct {
	ShortCode   string
	OriginalURL string
	CreatedAt   time.Time
}
