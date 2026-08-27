package mcp

import (
	"errors"
	"fmt"
)

var (
	ErrNilServer = errors.New("mcp: server must not be nil")

	ErrNilSession = errors.New("mcp: session must not be nil")
)

// ToolCallError is returned by tools produced by [DiscoverTools] when a remote MCP tool
// reports IsError=true. Use [errors.AsType] to distinguish a tool-side
// failure from transport, protocol, or argument-decoding errors:
//
//	out, err := tool.Call(ctx, args)
//	if tcErr, ok := errors.AsType[*mcp.ToolCallError](err); ok {
//	    // remote tool itself failed; surface tcErr.Message
//	} else if err != nil {
//	    // transport / argument failure; surface the infrastructure error
//	}
type ToolCallError struct {
	// RemoteName is the original MCP tool name as the server advertised
	// it (not the prefixed name reported into the registry).
	RemoteName string

	// Message is the human-readable failure text reported by the tool,
	// or a fallback when the tool returned IsError=true with no text.
	Message string
}

func (t *ToolCallError) Error() string {
	if t == nil {
		return "mcp tool call failed"
	}
	return fmt.Sprintf("mcp tool %q failed: %s", t.RemoteName, t.Message)
}
