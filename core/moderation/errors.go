package moderation

import "errors"

var (
	// ErrInvalidOptions reports malformed moderation options.
	ErrInvalidOptions = errors.New("moderation: invalid options")
	// ErrInvalidRequest reports a malformed moderation request.
	ErrInvalidRequest = errors.New("moderation: invalid request")
	// ErrInvalidResponse reports malformed moderation response data.
	ErrInvalidResponse = errors.New("moderation: invalid response")
)
