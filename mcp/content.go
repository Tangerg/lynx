package mcp

import (
	"encoding/json"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// emptyObjectSchema is the canonical "accepts an empty JSON object"
// schema — the fallback whenever a tool advertises no input schema.
const emptyObjectSchema = `{"type":"object"}`

// textOfContent returns the Text body of c, or "" when c has no textual
// representation (image / audio / embedded resource).
func textOfContent(c sdkmcp.Content) string {
	if t, ok := c.(*sdkmcp.TextContent); ok {
		return t.Text
	}
	return ""
}

// flattenContent reduces a CallToolResult.Content slice into the single
// string shape chat.CallableTool.Call must return:
//
//   - empty slice          → ""
//   - exactly one Text     → its Text verbatim
//   - everything else      → JSON of the whole slice (preserves the
//     "type" discriminator so the LLM still sees structure).
func flattenContent(items []sdkmcp.Content) (string, error) {
	switch {
	case len(items) == 0:
		return "", nil
	case len(items) == 1:
		if t, ok := items[0].(*sdkmcp.TextContent); ok {
			return t.Text, nil
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// firstTextOrFallback returns the first non-empty Text body in items, or
// fallback. Used to render a human-readable message for an MCP tool error.
func firstTextOrFallback(items []sdkmcp.Content, fallback string) string {
	for _, item := range items {
		if t := textOfContent(item); t != "" {
			return t
		}
	}
	return fallback
}

// decodeArguments parses the JSON argument blob into the typeless form
// CallToolParams.Arguments accepts. Empty input becomes {}.
func decodeArguments(arguments string) (any, error) {
	if arguments == "" {
		return map[string]any{}, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// schemaToString converts the heterogeneous sdkmcp.Tool.InputSchema
// (declared `any`) into the JSON string form lynx requires. Pre-encoded
// shapes pass through; everything else is JSON-marshaled. A missing
// schema falls back to [emptyObjectSchema].
func schemaToString(schema any) (string, error) {
	switch v := schema.(type) {
	case nil:
		return emptyObjectSchema, nil
	case string:
		if v == "" {
			return emptyObjectSchema, nil
		}
		return v, nil
	case json.RawMessage:
		if len(v) == 0 {
			return emptyObjectSchema, nil
		}
		return string(v), nil
	case []byte:
		if len(v) == 0 {
			return emptyObjectSchema, nil
		}
		return string(v), nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}
}

// stringSchemaToAny adapts a lynx ToolDefinition.InputSchema (always a
// JSON string) to the heterogeneous sdkmcp.Tool.InputSchema field
// (declared `any`). The SDK accepts json.RawMessage on the low-level
// AddTool path, which is exactly what we have.
func stringSchemaToAny(schema string) (any, error) {
	if schema == "" {
		return json.RawMessage(emptyObjectSchema), nil
	}
	if !json.Valid([]byte(schema)) {
		return nil, errors.New("schema is not valid JSON")
	}
	return json.RawMessage(schema), nil
}
