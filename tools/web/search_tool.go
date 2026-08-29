package web

import (
	"context"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

var _ toolcontract.Tool = (*SearchTool)(nil)

// SearchTool is the LLM-facing adapter for a [Searcher]. Construct
// with [NewSearchTool] — there is no nil-default fallback because web search
// inherently requires an upstream API.
type SearchTool struct {
	readOnlyTool
}

func NewSearchTool(searcher Searcher) (*SearchTool, error) {
	inner, err := newProviderReadOnlyTool(
		"search",
		toolcontract.FuncConfig{Name: "web_search", Description: webSearchDescription},
		searcher,
		ErrMissingSearcher,
		func(request SearchRequest) (*SearchRequest, error) { return request.Prepare() },
		func(ctx context.Context, request *SearchRequest) (*SearchResponse, error) {
			return searcher.Search(ctx, request)
		},
		func(response *SearchResponse) error { return response.Validate() },
	)
	if err != nil {
		return nil, err
	}
	return &SearchTool{readOnlyTool: inner}, nil
}

const webSearchDescription = `Search the web for current information.
- Returns a ranked list of result items, each with title, URL, and snippet
- Use this for events, products, prices, releases, people, docs — anything time-sensitive or beyond training data
- A single call is one search request; pass max_results to cap the size (configured default is typically 5-10)
- Domain filtering: allowed_domains restricts to those sites, blocked_domains excludes them. They are mutually exclusive
- Recency filter: pass "hour" / "day" / "week" / "month" / "year" when you need fresh results

CRITICAL — When you use this tool you MUST cite sources:
- After your answer, include a "Sources:" section
- List the URLs you used as markdown links: [Title](URL)
- Cite only URLs that actually appeared in the results — never fabricate

Search hygiene:
- For "latest X" queries, include the current year explicitly in the query string
- For official docs, restrict with allowed_domains (e.g. ["nodejs.org"]) — far less noise than open web
- If the first query returns weak hits, refine keywords and search again rather than guessing`
