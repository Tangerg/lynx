package websearch

import "errors"

// Sentinel errors surfaced by the tool and by provider impls.
var (
	// ErrMissingRequest is returned by [Request.Validate] when the
	// receiver is nil.
	ErrMissingRequest = errors.New("websearch: request must not be nil")

	// ErrEmptyQuery is returned when [Request.Query] is empty.
	ErrEmptyQuery = errors.New("websearch: query must not be empty")

	// ErrDomainsBothSides is returned when both AllowedDomains and
	// BlockedDomains are set — most providers only honor one.
	ErrDomainsBothSides = errors.New("websearch: allowed_domains and blocked_domains are mutually exclusive")

	// ErrMissingProvider is returned by [NewTool] when the provider
	// argument is nil.
	ErrMissingProvider = errors.New("websearch: provider is required")
)
