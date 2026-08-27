package embedding

import "errors"

var (
	ErrInvalidOptions  = errors.New("embedding: invalid options")
	ErrInvalidRequest  = errors.New("embedding: invalid request")
	ErrInvalidResponse = errors.New("embedding: invalid response")
)
