package web

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Recency is a coarse "last N period" filter. Providers map this to
// their native syntax (e.g. Tavily's time_range, Serper's tbs=qdr:).
type Recency string

const (
	RecencyHour  Recency = "hour"
	RecencyDay   Recency = "day"
	RecencyWeek  Recency = "week"
	RecencyMonth Recency = "month"
	RecencyYear  Recency = "year"
)

// Validate reports whether the recency is empty or one of the supported
// coarse windows.
func (r Recency) Validate() error {
	switch r {
	case "", RecencyHour, RecencyDay, RecencyWeek, RecencyMonth, RecencyYear:
		return nil
	default:
		return ErrInvalidRecency
	}
}

// SearchRequest is both the provider-neutral search contract and the
// LLM-facing argument shape.
type SearchRequest struct {
	// Query is the search string. Required.
	Query string `json:"query" jsonschema:"minLength=1" jsonschema_description:"Non-empty web search query. Include the current year when asking for the latest information."`

	// MaxResults caps the number of returned results. 0 = use the
	// provider's default (typically 5-10).
	MaxResults int `json:"max_results,omitempty" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Maximum results to return, from 1 to 20. Omit to use the configured search default (typically 5-10)."`

	// AllowedDomains restricts results to these domains. Mutually
	// exclusive with BlockedDomains on most providers.
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema:"maxItems=20" jsonschema_description:"Only include results from at most 20 domains (bare domain names, no protocol). Mutually exclusive with blocked_domains."`

	// BlockedDomains drops results from these domains.
	BlockedDomains []string `json:"blocked_domains,omitempty" jsonschema:"maxItems=20" jsonschema_description:"Exclude results from at most 20 domains (bare domain names, no protocol). Mutually exclusive with allowed_domains."`

	// Recency filters to a coarse time-window. "" = no time filter.
	Recency Recency `json:"recency,omitempty" jsonschema:"enum=hour,enum=day,enum=week,enum=month,enum=year" jsonschema_description:"Optional time window: hour, day, week, month, or year."`
}

// Prepare returns an independently owned, normalized request after validating
// it. The receiver is never mutated.
func (s *SearchRequest) Prepare() (*SearchRequest, error) {
	if s == nil {
		return nil, ErrMissingSearchRequest
	}
	prepared := *s
	prepared.Query = strings.TrimSpace(s.Query)
	prepared.AllowedDomains = slices.Clone(s.AllowedDomains)
	prepared.BlockedDomains = slices.Clone(s.BlockedDomains)
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return &prepared, nil
}

// Validate checks the cross-cutting invariants the tool and every
// provider enforce: non-nil, non-empty query, and domain allow/block
// mutual exclusion. Returns one of the sentinel errors in errors.go
// so callers can match with errors.Is.
func (s *SearchRequest) Validate() error {
	if s == nil {
		return ErrMissingSearchRequest
	}
	if strings.TrimSpace(s.Query) == "" {
		return ErrEmptyQuery
	}
	if s.MaxResults < 0 || s.MaxResults > 20 {
		return ErrInvalidMaxResults
	}
	if len(s.AllowedDomains) > 0 && len(s.BlockedDomains) > 0 {
		return ErrDomainsBothSides
	}
	if len(s.AllowedDomains) > 20 || len(s.BlockedDomains) > 20 {
		return ErrTooManyDomains
	}
	return s.Recency.Validate()
}

// QueryWithSiteOperators returns Query with Google-style site:/-site:
// operators for the request's domain filters. Providers without native domain
// fields use this projection; empty domain entries are ignored.
func (s *SearchRequest) QueryWithSiteOperators() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(s.Query)
	for _, domain := range s.AllowedDomains {
		if domain != "" {
			fmt.Fprintf(&b, " site:%s", domain)
		}
	}
	for _, domain := range s.BlockedDomains {
		if domain != "" {
			fmt.Fprintf(&b, " -site:%s", domain)
		}
	}
	return b.String()
}

// SearchResult is one normalized search hit.
type SearchResult struct {
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	Snippet       string    `json:"snippet"`
	FaviconURL    string    `json:"favicon_url,omitempty"`
	PublishedTime time.Time `json:"published_time,omitzero"`
	Source        string    `json:"source,omitempty"`
}

// SearchResponse carries the executed query plus normalized results. Used
// as both the SPI return type and the LLM-facing serialization shape.
type SearchResponse struct {
	Query   string          `json:"query"`
	Results []*SearchResult `json:"results"`
}

// Searcher is the SPI a search backend implements.
type Searcher interface {
	// Search performs a single search and returns normalized results.
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
}
