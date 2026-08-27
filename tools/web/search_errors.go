package web

import "errors"

var (
	ErrMissingSearchRequest = errors.New("web: search request must not be nil")

	ErrEmptyQuery = errors.New("web: search query must not be empty")

	ErrDomainsBothSides  = errors.New("web: allowed_domains and blocked_domains are mutually exclusive")
	ErrInvalidMaxResults = errors.New("web: max_results must be between 1 and 20 when set")
	ErrTooManyDomains    = errors.New("web: at most 20 allowed_domains or blocked_domains may be set")
	ErrInvalidRecency    = errors.New("web: recency must be hour, day, week, month, or year")

	ErrMissingSearcher = errors.New("web: searcher is required")
)
