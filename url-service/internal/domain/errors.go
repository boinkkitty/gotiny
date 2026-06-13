package domain

import "errors"

var (
	ErrNotFound = errors.New("url not found")
	ErrNotOwner = errors.New("not the owner of this url")
)
