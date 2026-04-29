package mcp

import "fmt"

// ToolCallError is returned by Tool.Call when a remote MCP tool reports
// IsError=true. Extract it with errors.As to distinguish a tool-side
// failure from transport, protocol, or argument-decoding errors:
//
//	out, err := tool.Call(ctx, args)
//	var tcErr *lynxmcp.ToolCallError
//	switch {
//	case errors.As(err, &tcErr):
//	    // remote tool itself failed; surface message to user / LLM
//	case err != nil:
//	    // transport or argument failure; retry / give up
//	}
type ToolCallError struct {
	// ToolName is the original MCP tool name as the server advertised it
	// (not the prefixed name reported into the lynx registry).
	ToolName string

	// Message is the human-readable failure text reported by the tool, or
	// a fallback when the tool returned IsError=true with no text content.
	Message string
}

// Error implements error.
func (e *ToolCallError) Error() string {
	return fmt.Sprintf("mcp tool %q failed: %s", e.ToolName, e.Message)
}
