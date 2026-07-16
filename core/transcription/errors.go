package transcription

import "errors"

var (
	// ErrInvalidOptions reports malformed transcription options.
	ErrInvalidOptions = errors.New("transcription: invalid options")
	// ErrInvalidRequest reports a malformed transcription request.
	ErrInvalidRequest = errors.New("transcription: invalid request")
	// ErrInvalidResponse reports malformed transcription response data.
	ErrInvalidResponse = errors.New("transcription: invalid response")
)
