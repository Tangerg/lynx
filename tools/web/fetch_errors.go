package web

import "errors"

// Sentinel errors surfaced by the tool and by provider impls.
var (
	// ErrMissingFetchRequest is returned by [FetchRequest.Validate] when the
	// receiver is nil.
	ErrMissingFetchRequest = errors.New("web: fetch request must not be nil")

	// ErrEmptyURL is returned when [FetchRequest.URL] is empty.
	ErrEmptyURL      = errors.New("web: fetch URL must not be empty")
	ErrInvalidURL    = errors.New("web: fetch URL must be an absolute http(s) URL")
	ErrInvalidFormat = errors.New("web: content format must be markdown, html, or text")

	// ErrMissingFetcher is returned by [NewFetchTool] when the provider
	// argument is nil.
	ErrMissingFetcher = errors.New("web: fetcher is required")
)
