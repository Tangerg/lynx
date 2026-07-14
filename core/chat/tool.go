package chat

import "fmt"

// ToolCall is a model request to invoke a named tool. Arguments retains the
// provider's JSON text instead of json.RawMessage so malformed model output and
// streaming fragments remain serializable; the tool runtime owns decoding.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// Validate verifies call identity. Arguments is intentionally not parsed here:
// model output may be partial or malformed and still needs to round-trip.
func (c ToolCall) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("%w: empty ID", ErrInvalidToolCall)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidToolCall)
	}
	return nil
}

// ToolResult is one tool execution result correlated to a ToolCall by ID.
type ToolResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error,omitempty"`
}

// Validate verifies result identity. An empty Result is valid because a tool
// may complete without producing output.
func (r ToolResult) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("%w: empty ID", ErrInvalidToolResult)
	}
	if r.Name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidToolResult)
	}
	return nil
}
