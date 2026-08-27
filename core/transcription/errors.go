package transcription

import "errors"

var (
	ErrInvalidOptions  = errors.New("transcription: invalid options")
	ErrInvalidRequest  = errors.New("transcription: invalid request")
	ErrInvalidResponse = errors.New("transcription: invalid response")
)
