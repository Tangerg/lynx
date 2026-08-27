package speech

import "errors"

var (
	ErrInvalidOptions  = errors.New("speech: invalid options")
	ErrInvalidRequest  = errors.New("speech: invalid request")
	ErrInvalidResponse = errors.New("speech: invalid response")
)
