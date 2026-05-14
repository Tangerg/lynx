package webfetch

import "errors"

// Sentinel errors surfaced by the shell and by provider impls.
var (
	// ErrMissingRequest is returned by [Request.Validate] when the
	// receiver is nil.
	ErrMissingRequest = errors.New("webfetch: request must not be nil")

	// ErrEmptyURL is returned when [Request.URL] is empty.
	ErrEmptyURL = errors.New("webfetch: url must not be empty")

	// ErrMissingProvider is returned by [NewTool] when the provider
	// argument is nil.
	ErrMissingProvider = errors.New("webfetch: provider is required")
)
