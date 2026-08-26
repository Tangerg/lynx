package web

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/core/tool"
)

var _ toolcontract.Tool = (*SearchTool)(nil)

// SearchTool is the LLM-facing adapter for a [Searcher]. Construct
// with [NewSearchTool] — there is no nil-default fallback because web search
// inherently requires an upstream API.
type SearchTool struct {
	searcher Searcher
	inner    toolcontract.Tool
}

// NewSearchTool builds a [SearchTool] backed by searcher. It returns an error
// when searcher is nil because there is no sensible
// local fallback.
func NewSearchTool(searcher Searcher) (*SearchTool, error) {
	if searcher == nil {
		return nil, ErrMissingSearcher
	}
	t := &SearchTool{searcher: searcher}
	inner, err := toolcontract.NewFunc[SearchRequest, *SearchResponse](
		toolcontract.FuncConfig{Name: "web_search", Description: webSearchDescription},
		t.search,
	)
	if err != nil {
		return nil, fmt.Errorf("web: build search tool: %w", err)
	}
	t.inner = inner
	return t, nil
}

func (t *SearchTool) Definition() chat.ToolDefinition { return t.inner.Definition() }

// webSearchDescription is the LLM-facing prompt. Structure follows
// the standard WebSearch prompt: short bullets + a CRITICAL block
// for the source-citation contract.
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

// ConcurrencyKey opts web_search into parallel execution — a read-only search
// has no local resource conflict (the tool loop's optional concurrency
// contract), so the loop runs several searches at once.
func (t *SearchTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *SearchTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.inner.Call(ctx, arguments)
}

func (t *SearchTool) search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("web: search: %w", err)
	}

	res, err := t.searcher.Search(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("web: search: %w", err)
	}
	return res, nil
}
