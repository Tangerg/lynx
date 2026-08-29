package web

import "errors"

var (
	ErrMissingFetchRequest = errors.New("web: fetch request must not be nil")

	ErrEmptyURL      = errors.New("web: fetch URL must not be empty")
	ErrInvalidURL    = errors.New("web: fetch URL must be an absolute http(s) URL")
	ErrInvalidFormat = errors.New("web: content format must be markdown, html, or text")

	ErrMissingFetcher       = errors.New("web: fetcher is required")
	ErrMissingFetchResponse = errors.New("web: fetch response must not be nil")
	ErrInvalidFetchResponse = errors.New("web: fetch response is invalid")
	ErrUnsupportedFormat    = errors.New("web: content format is not supported by provider")
)
