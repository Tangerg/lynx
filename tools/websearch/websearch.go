package websearch

import (
	"context"
	"fmt"
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

// Request is the shape every [Provider] consumes AND the LLM-facing
// argument shape — the two were identical so they're one type now.
// The JSON / jsonschema tags drive [Tool]'s input schema; provider
// impls read the Go fields directly.
type Request struct {
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

// Validate checks the cross-cutting invariants the tool and every
// provider enforce: non-nil, non-empty query, and domain allow/block
// mutual exclusion. Returns one of the sentinel errors in errors.go
// so callers can match with errors.Is.
func (r *Request) Validate() error {
	if r == nil {
		return ErrMissingRequest
	}
	r.Query = strings.TrimSpace(r.Query)
	if r.Query == "" {
		return ErrEmptyQuery
	}
	if r.MaxResults < 0 || r.MaxResults > 20 {
		return ErrInvalidMaxResults
	}
	if len(r.AllowedDomains) > 0 && len(r.BlockedDomains) > 0 {
		return ErrDomainsBothSides
	}
	if len(r.AllowedDomains) > 20 || len(r.BlockedDomains) > 20 {
		return ErrTooManyDomains
	}
	return r.Recency.Validate()
}

// QueryWithSiteOperators returns Query with Google-style site:/-site:
// operators for the request's domain filters. Providers without native domain
// fields use this projection; empty domain entries are ignored.
func (r *Request) QueryWithSiteOperators() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(r.Query)
	for _, domain := range r.AllowedDomains {
		if domain != "" {
			fmt.Fprintf(&b, " site:%s", domain)
		}
	}
	for _, domain := range r.BlockedDomains {
		if domain != "" {
			fmt.Fprintf(&b, " -site:%s", domain)
		}
	}
	return b.String()
}

// Result is one normalized search hit.
type Result struct {
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	Snippet       string    `json:"snippet"`
	FaviconURL    string    `json:"favicon_url,omitempty"`
	PublishedTime time.Time `json:"published_time,omitzero"`
	Source        string    `json:"source,omitempty"`
}

// Response carries the executed query plus normalized results. Used
// as both the SPI return type and the LLM-facing serialization shape.
type Response struct {
	Query   string    `json:"query"`
	Results []*Result `json:"results"`
}

// Provider is the SPI a search backend implements.
type Provider interface {
	// Name returns the provider's identifier (e.g. "tavily", "jina").
	Name() string

	// Search performs a single search and returns normalized results.
	Search(ctx context.Context, req *Request) (*Response, error)
}
