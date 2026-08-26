package web

import "errors"

// Sentinel errors surfaced by the tool and by provider impls.
var (
	// ErrMissingSearchRequest is returned by [SearchRequest.Validate] when the
	// receiver is nil.
	ErrMissingSearchRequest = errors.New("web: search request must not be nil")

	// ErrEmptyQuery is returned when [SearchRequest.Query] is empty.
	ErrEmptyQuery = errors.New("web: search query must not be empty")

	// ErrDomainsBothSides is returned when both AllowedDomains and
	// BlockedDomains are set — most providers only honor one.
	ErrDomainsBothSides  = errors.New("web: allowed_domains and blocked_domains are mutually exclusive")
	ErrInvalidMaxResults = errors.New("web: max_results must be between 1 and 20 when set")
	ErrTooManyDomains    = errors.New("web: at most 20 allowed_domains or blocked_domains may be set")
	ErrInvalidRecency    = errors.New("web: recency must be hour, day, week, month, or year")

	// ErrMissingSearcher is returned by [NewSearchTool] when the provider
	// argument is nil.
	ErrMissingSearcher = errors.New("web: searcher is required")
)
