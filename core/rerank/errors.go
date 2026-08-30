package rerank

import "errors"

var (
	ErrInvalidOptions  = errors.New("rerank: invalid options")
	ErrInvalidRequest  = errors.New("rerank: invalid request")
	ErrInvalidResponse = errors.New("rerank: invalid response")
)
