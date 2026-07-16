package speech

import "errors"

var (
	// ErrInvalidOptions reports malformed speech-generation options.
	ErrInvalidOptions = errors.New("speech: invalid options")
	// ErrInvalidRequest reports a malformed speech-generation request.
	ErrInvalidRequest = errors.New("speech: invalid request")
	// ErrInvalidResponse reports malformed speech-generation response data.
	ErrInvalidResponse = errors.New("speech: invalid response")
)
