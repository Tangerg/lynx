package mcp

import "fmt"

// ToolCallError is returned by [Tool.Call] when a remote MCP tool
// reports IsError=true. Use errors.As to distinguish a tool-side
// failure from transport, protocol, or argument-decoding errors:
//
//	out, err := tool.Call(ctx, args)
//	var tcErr *mcp.ToolCallError
//	switch {
//	case errors.As(err, &tcErr):
//	    // remote tool itself failed; surface tcErr.Message
//	case err != nil:
//	    // transport / argument failure; retry or alert
//	}
type ToolCallError struct {
	// ToolName is the original MCP tool name as the server advertised
	// it (not the prefixed name reported into the lynx registry).
	ToolName string

	// Message is the human-readable failure text reported by the tool,
	// or a fallback when the tool returned IsError=true with no text.
	Message string
}

func (e *ToolCallError) Error() string {
	return fmt.Sprintf("mcp tool %q failed: %s", e.ToolName, e.Message)
}
