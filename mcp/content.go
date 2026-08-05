package mcp

import (
	"encoding/json"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// emptyObjectSchema is the canonical "accepts an empty JSON object"
// schema — the fallback whenever a tool advertises no input schema.
const emptyObjectSchema = `{"type":"object","additionalProperties":false}`

func textOfContent(c sdkmcp.Content) string {
	if t, ok := c.(*sdkmcp.TextContent); ok {
		return t.Text
	}
	return ""
}

// flattenContent reduces a [sdkmcp.CallToolResult.Content] slice into
// the single string shape [chat.Tool.Call] must return:
//
//   - empty slice      → ""
//   - exactly one Text → its Text verbatim
//   - everything else  → JSON of the whole slice (preserves the
//     "type" discriminator so the LLM still sees structure).
func flattenContent(items []sdkmcp.Content) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	if len(items) == 1 {
		if t, ok := items[0].(*sdkmcp.TextContent); ok {
			return t.Text, nil
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("mcp: encode tool content: %w", err)
	}
	return string(encoded), nil
}

func firstTextOrFallback(items []sdkmcp.Content, fallback string) string {
	for _, item := range items {
		if t := textOfContent(item); t != "" {
			return t
		}
	}
	return fallback
}

func decodeArguments(arguments string) (any, error) {
	if arguments == "" {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return nil, fmt.Errorf("mcp: decode tool arguments: %w", err)
	}
	if decoded == nil {
		return nil, errors.New("mcp: tool arguments must be a JSON object")
	}
	return decoded, nil
}

// schemaToJSON converts the heterogeneous [sdkmcp.Tool.InputSchema]
// (declared `any`) into the raw JSON form the framework requires.
// Pre-encoded shapes pass through unchanged; everything else is
// JSON-marshaled. A missing or empty schema falls back to
// [emptyObjectSchema].
func schemaToJSON(schema any) (json.RawMessage, error) {
	switch v := schema.(type) {
	case nil:
		return append(json.RawMessage(nil), emptyObjectSchema...), nil
	case string:
		if v == "" {
			return append(json.RawMessage(nil), emptyObjectSchema...), nil
		}
		return json.RawMessage(v), nil
	case json.RawMessage:
		if len(v) == 0 {
			return append(json.RawMessage(nil), emptyObjectSchema...), nil
		}
		return append(json.RawMessage(nil), v...), nil
	case []byte:
		if len(v) == 0 {
			return append(json.RawMessage(nil), emptyObjectSchema...), nil
		}
		return append(json.RawMessage(nil), v...), nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("mcp: encode tool input schema: %w", err)
		}
		return encoded, nil
	}
}
