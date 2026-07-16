package embedding

import "errors"

var (
	// ErrInvalidOptions reports malformed embedding options.
	ErrInvalidOptions = errors.New("embedding: invalid options")
	// ErrInvalidRequest reports a malformed embedding request.
	ErrInvalidRequest = errors.New("embedding: invalid request")
	// ErrInvalidResponse reports malformed embedding response data.
	ErrInvalidResponse = errors.New("embedding: invalid response")
)
