package httpreq

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

var _ toolcontract.Tool = (*Tool)(nil)

// Tool is the LLM-facing adapter for [Client].
type Tool struct {
	client *Client
	inner  toolcontract.Tool
}

// NewTool builds a [Tool] backed by client. Returns an error if
// client is nil — there is no nil-default because the allowlist must
// be configured explicitly.
func NewTool(client *Client) (*Tool, error) {
	if client == nil {
		return nil, errors.New("httpreq: client is required")
	}
	t := &Tool{client: client}
	inner, err := toolcontract.NewFunc[Request, *Response](
		toolcontract.FuncConfig{Name: "http_request", Description: description},
		t.request,
	)
	if err != nil {
		return nil, fmt.Errorf("httpreq: build tool: %w", err)
	}
	t.inner = inner
	return t, nil
}

func (t *Tool) Definition() chat.ToolDefinition { return t.inner.Definition() }

const description = `Execute a single HTTP request and return the response.
- The "url" must be a fully-formed absolute http(s) URL.
- Method defaults to GET. Write methods (POST/PUT/PATCH/DELETE) only work when configured policy allows them.
- Configured policy restricts which hosts and methods are reachable. A policy rejection is final for that host and method; do not retry the same request.
- Response body is capped (default 256 KiB); when truncated, response.truncated == true.
- For body with JSON content, pass a JSON-encoded string as "body" and set Content-Type via "headers".
- Use this for arbitrary REST/JSON APIs. Prefer the dedicated web_search / web_fetch tools for general web pages.`

func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	return t.inner.Call(ctx, arguments)
}

func (t *Tool) request(ctx context.Context, req Request) (*Response, error) {
	res, err := t.client.Do(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("httpreq: %w", err)
	}
	return res, nil
}
