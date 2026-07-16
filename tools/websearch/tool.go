package websearch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	toolcontract "github.com/Tangerg/lynx/tools"
)

var toolSchema, _ = pkgjson.StringDefSchemaOf(Request{})

var _ toolcontract.Tool = (*Tool)(nil)

// Tool is the LLM-facing adapter for a websearch [Provider]. Construct
// with [NewTool] — there is no nil-default fallback because web search
// inherently requires an upstream API.
type Tool struct {
	provider Provider
}

// NewTool builds a [Tool] backed by provider. Returns an error if
// provider is nil — unlike the shell/fs tools, there is no sensible
// local fallback.
func NewTool(provider Provider) (*Tool, error) {
	if provider == nil {
		return nil, ErrMissingProvider
	}
	return &Tool{provider: provider}, nil
}

func (t *Tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "web_search",
		Description: webSearchDescription,
		InputSchema: json.RawMessage(toolSchema),
	}
}

// webSearchDescription is the LLM-facing prompt. Structure follows
// the standard WebSearch prompt: short bullets + a CRITICAL block
// for the source-citation contract.
const webSearchDescription = `Search the web for current information.
- Returns a ranked list of result items, each with title, URL, and snippet
- Use this for events, products, prices, releases, people, docs — anything time-sensitive or beyond training data
- A single call is one API round-trip; pass max_results to cap the size (default 5-10 per provider)
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
func (t *Tool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	var req Request
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("websearch: parse arguments: %w", err)
	}
	if err := req.Validate(); err != nil {
		return "", fmt.Errorf("websearch: %w", err)
	}

	res, err := t.provider.Search(ctx, &req)
	if err != nil {
		return "", fmt.Errorf("websearch: %w", err)
	}
	body, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("websearch: marshal: %w", err)
	}
	return string(body), nil
}
