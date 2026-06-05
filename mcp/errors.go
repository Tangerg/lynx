package mcp

import (
	"errors"
	"fmt"
)

// Sentinel errors for the input-shape validators. Callers can match
// these with [errors.Is] to distinguish caller-side input errors from
// transport, protocol, or remote-tool failures.
var (
	// ErrNilServer is returned by [RegisterTools] when server is nil.
	ErrNilServer = errors.New("mcp: server must not be nil")

	// ErrNilSession is returned when a [Source] or [ToolConfig]
	// supplies a nil session.
	ErrNilSession = errors.New("mcp: session must not be nil")

	// ErrNilDescriptor is returned when [ToolConfig] supplies a nil
	// tool descriptor.
	ErrNilDescriptor = errors.New("mcp: descriptor must not be nil")
)

// ToolCallError is returned by [Tool.Call] when a remote MCP tool
// reports IsError=true. Use [errors.As] to distinguish a tool-side
// failure from transport, protocol, or argument-decoding errors:
//
//	out, err := tool.Call(ctx, args)
//	if tcErr, ok := errors.AsType[*mcp.ToolCallError](err); ok {
//	    // remote tool itself failed; surface tcErr.Message
//	} else if err != nil {
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
