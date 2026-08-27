package web

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

var _ toolcontract.Tool = (*FetchTool)(nil)

// FetchTool is the LLM-facing adapter for a [Fetcher]. Construct
// with [NewFetchTool] — there is no nil-default fallback because rendering
// modern web pages reliably requires an upstream API.
type FetchTool struct {
	fetcher Fetcher
	inner   toolcontract.Tool
}

func NewFetchTool(fetcher Fetcher) (*FetchTool, error) {
	if fetcher == nil {
		return nil, ErrMissingFetcher
	}
	t := &FetchTool{fetcher: fetcher}
	inner, err := toolcontract.NewFunc[FetchRequest, *FetchResponse](
		toolcontract.FuncConfig{Name: "web_fetch", Description: webFetchDescription},
		t.fetch,
	)
	if err != nil {
		return nil, fmt.Errorf("web: build fetch tool: %w", err)
	}
	t.inner = inner
	return t, nil
}

func (f *FetchTool) Definition() chat.ToolDefinition { return f.inner.Definition() }

// webFetchDescription is the LLM-facing prompt. Structure follows
// the standard WebFetch prompt.
const webFetchDescription = `Fetch and read a single web page, returning the content in a clean format.
- Takes a fully-formed http(s) URL
- Returns the page content rendered to the requested format (markdown by default)
- Use this after web_search when result snippets don't contain enough detail
- Use this when the user gives you a specific URL
- For JS-heavy / SPA pages, prefer this tool over shell + curl — rendering is handled automatically

Format options:
- "markdown" (default) — best for readable structured content
- "html" — when you need DOM structure or specific elements
- "text" — plain text, no markup

Usage notes:
- The tool is read-only; it never modifies files
- This tool WILL FAIL on authenticated or private URLs (Google Docs, Confluence, Jira, internal wikis) — look for an authenticated integration tool
- For GitHub URLs, prefer shell + the gh CLI (gh pr view / gh issue view / gh api) — it handles auth and pagination properly
- If you get a redirect or 4xx error, the URL is likely wrong, gated, or expired — don't retry blindly`

// ConcurrencyKey opts web_fetch into parallel execution — a read-only network
// fetch has no local resource conflict (the tool loop's optional concurrency
// contract), so the loop fetches several URLs at once.
func (f *FetchTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (f *FetchTool) Call(ctx context.Context, arguments string) (string, error) {
	return f.inner.Call(ctx, arguments)
}

func (f *FetchTool) fetch(ctx context.Context, req FetchRequest) (*FetchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("web: fetch: %w", err)
	}

	res, err := f.fetcher.Fetch(ctx, prepared)
	if err != nil {
		return nil, fmt.Errorf("web: fetch: %w", err)
	}
	return res, nil
}
