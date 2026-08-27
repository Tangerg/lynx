package web

import (
	"context"
	"fmt"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

var _ toolcontract.Tool = (*FetchTool)(nil)

// FetchTool is the LLM-facing adapter for a [Fetcher]. Construct
// with [NewFetchTool] — there is no nil-default fallback because rendering
// modern web pages reliably requires an upstream API.
type FetchTool struct {
	readOnlyTool
}

func NewFetchTool(fetcher Fetcher) (*FetchTool, error) {
	if fetcher == nil {
		return nil, ErrMissingFetcher
	}
	t := &FetchTool{}
	if err := t.bind(
		toolcontract.FuncConfig{Name: "web_fetch", Description: webFetchDescription},
		func(ctx context.Context, request FetchRequest) (*FetchResponse, error) {
			prepared, err := request.Prepare()
			if err != nil {
				return nil, fmt.Errorf("web: prepare fetch request: %w", err)
			}
			response, err := fetcher.Fetch(ctx, prepared)
			if err != nil {
				return nil, fmt.Errorf("web: execute fetch: %w", err)
			}
			return response, nil
		},
	); err != nil {
		return nil, fmt.Errorf("web: build fetch tool: %w", err)
	}
	return t, nil
}

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
