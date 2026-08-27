package moderation

import "errors"

var (
	ErrInvalidOptions  = errors.New("moderation: invalid options")
	ErrInvalidRequest  = errors.New("moderation: invalid request")
	ErrInvalidResponse = errors.New("moderation: invalid response")
)
