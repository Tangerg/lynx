package image

import "errors"

var (
	ErrInvalidOptions  = errors.New("image: invalid options")
	ErrInvalidRequest  = errors.New("image: invalid request")
	ErrInvalidResponse = errors.New("image: invalid response")
)
