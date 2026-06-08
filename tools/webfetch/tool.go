package webfetch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

var toolSchema, _ = pkgjson.StringDefSchemaOf(Request{})

var _ chat.Tool = (*Tool)(nil)

// Tool is the LLM-facing adapter for a webfetch [Provider]. Construct
// with [NewTool] — there is no nil-default fallback because rendering
// modern web pages reliably requires an upstream API.
type Tool struct {
	provider Provider
}

// NewTool builds a [Tool] backed by provider. Returns an error if
// provider is nil.
func NewTool(provider Provider) (*Tool, error) {
	if provider == nil {
		return nil, ErrMissingProvider
	}
	return &Tool{provider: provider}, nil
}

func (t *Tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "web_fetch",
		Description: webFetchDescription,
		InputSchema: toolSchema,
	}
}

// webFetchDescription is the LLM-facing prompt. Structure follows
// Claude Code's WebFetch prompt.
const webFetchDescription = `Fetch and read a single web page, returning the content in a clean format.
- Takes a fully-formed http(s) URL
- Returns the page content rendered to the requested format (markdown by default)
- Use this after web_search when result snippets don't contain enough detail
- Use this when the user gives you a specific URL
- For JS-heavy / SPA pages, prefer this tool over bash + curl — the provider handles rendering

Format options:
- "markdown" (default) — best for LLM context; structure preserved
- "html" — when you need DOM structure or specific elements
- "text" — plain text, no markup

Usage notes:
- The tool is read-only; it never modifies files
- HTTP URLs are upgraded to HTTPS automatically by most providers
- This tool WILL FAIL on authenticated or private URLs (Google Docs, Confluence, Jira, internal wikis) — look for a specialized MCP tool that does authenticated access
- For GitHub URLs, prefer bash + the gh CLI (gh pr view / gh issue view / gh api) — it handles auth and pagination properly
- If you get a redirect or 4xx error, the URL is likely wrong, gated, or expired — don't retry blindly`

func (t *Tool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req Request
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("webfetch: parse arguments: %w", err)
	}
	if err := req.Validate(); err != nil {
		return "", fmt.Errorf("webfetch: %w", err)
	}

	res, err := t.provider.Fetch(ctx, &req)
	if err != nil {
		return "", fmt.Errorf("webfetch: %w", err)
	}
	body, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("webfetch: marshal: %w", err)
	}
	return string(body), nil
}
