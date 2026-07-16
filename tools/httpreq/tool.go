package httpreq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	toolcontract "github.com/Tangerg/lynx/tools"
)

var toolSchema, _ = pkgjson.StringDefSchemaOf(Request{})

var _ toolcontract.Tool = (*Tool)(nil)

// Tool is the LLM-facing adapter for [Client].
type Tool struct {
	client *Client
}

// NewTool builds a [Tool] backed by client. Returns an error if
// client is nil — there is no nil-default because the allowlist must
// be configured explicitly.
func NewTool(client *Client) (*Tool, error) {
	if client == nil {
		return nil, errors.New("httpreq: client is required")
	}
	return &Tool{client: client}, nil
}

func (t *Tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "http_request",
		Description: description,
		InputSchema: json.RawMessage(toolSchema),
	}
}

const description = `Execute a single HTTP request and return the response.
- The "url" must be a fully-formed absolute http(s) URL.
- Method defaults to GET. Write methods (POST/PUT/PATCH/DELETE) only work if the host has been configured to allow them.
- The agent operator restricts which hosts and methods are reachable; if you get a "host is not in AllowedHosts" or "method is not in AllowedMethods" error, the request is permanently blocked — don't retry with the same host/method.
- Response body is capped (default 256 KiB); when truncated, response.truncated == true.
- For body with JSON content, pass a JSON-encoded string as "body" and set Content-Type via "headers".
- Use this for arbitrary REST/JSON APIs. Prefer the dedicated web_search / web_fetch tools for general web pages.`

func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	var req Request
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("httpreq: parse arguments: %w", err)
	}

	res, err := t.client.Do(ctx, &req)
	if err != nil {
		return "", fmt.Errorf("httpreq: %w", err)
	}
	body, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("httpreq: marshal: %w", err)
	}
	return string(body), nil
}
