package websearch

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BuildSiteOperatorQuery inlines Google-style site:/-site: operators
// into a query string. Providers that have no native domain
// allow/block fields (Brave, Serper, Firecrawl search) use this to
// translate [Request.AllowedDomains] / [Request.BlockedDomains] into
// query-level filters.
//
// Empty strings inside the slices are skipped; the original query
// is preserved as-is in front.
func BuildSiteOperatorQuery(query string, allowed, blocked []string) string {
	var b strings.Builder
	b.WriteString(query)
	for _, s := range allowed {
		if s == "" {
			continue
		}
		fmt.Fprintf(&b, " site:%s", s)
	}
	for _, s := range blocked {
		if s == "" {
			continue
		}
		fmt.Fprintf(&b, " -site:%s", s)
	}
	return b.String()
}

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

// Request is the shape every [Provider] consumes AND the LLM-facing
// argument shape — the two were identical so they're one type now.
// The JSON / jsonschema tags drive [Tool]'s input schema; provider
// impls read the Go fields directly.
type Request struct {
	// Query is the search string. Required.
	Query string `json:"query" jsonschema:"required" jsonschema_description:"The search query (at least 2 characters)."`

	// MaxResults caps the number of returned results. 0 = use the
	// provider's default (typically 5-10).
	MaxResults int `json:"max_results,omitempty" jsonschema_description:"Number of results to return. 0 or omitted uses the provider default (typically 5-10)."`

	// AllowedDomains restricts results to these domains. Mutually
	// exclusive with BlockedDomains on most providers.
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema_description:"Only include results from these domains (bare domain names, no protocol). Mutually exclusive with blocked_domains."`

	// BlockedDomains drops results from these domains.
	BlockedDomains []string `json:"blocked_domains,omitempty" jsonschema_description:"Exclude results from these domains. Mutually exclusive with allowed_domains."`

	// Recency filters to a coarse time-window. "" = no time filter.
	Recency Recency `json:"recency,omitempty" jsonschema_description:"Time-window filter: \"hour\", \"day\", \"week\", \"month\", or \"year\". Useful for time-sensitive queries (news, releases, prices)."`
}

// Validate checks the cross-cutting invariants the shell and every
// provider enforce: non-nil, non-empty query, and domain allow/block
// mutual exclusion. Returns one of the sentinel errors in errors.go
// so callers can match with errors.Is.
func (r *Request) Validate() error {
	if r == nil {
		return ErrMissingRequest
	}
	if r.Query == "" {
		return ErrEmptyQuery
	}
	if len(r.AllowedDomains) > 0 && len(r.BlockedDomains) > 0 {
		return ErrDomainsBothSides
	}
	return nil
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
