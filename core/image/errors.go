package image

import "errors"

var (
	// ErrInvalidOptions reports malformed image-generation options.
	ErrInvalidOptions = errors.New("image: invalid options")
	// ErrInvalidRequest reports a malformed image-generation request.
	ErrInvalidRequest = errors.New("image: invalid request")
	// ErrInvalidResponse reports malformed image-generation response data.
	ErrInvalidResponse = errors.New("image: invalid response")
)
