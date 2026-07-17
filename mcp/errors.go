package mcp

import (
	"errors"
	"fmt"
)

// Sentinel errors for public input-shape validators. Callers can match these
// with [errors.Is] to distinguish caller-side input errors from transport,
// protocol, or remote-tool failures.
var (
	// ErrNilServer is returned by [Register] when server is nil.
	ErrNilServer = errors.New("mcp: server must not be nil")

	// ErrNilSession is returned when a [ToolSource] supplies a nil session.
	ErrNilSession = errors.New("mcp: session must not be nil")

	// ErrNilChatCaller is returned by [NewSamplingHandler] when caller is nil
	// or holds a typed nil value.
	ErrNilChatCaller = errors.New("mcp: chat caller must not be nil")
)

var errNilDescriptor = errors.New("mcp: descriptor must not be nil")

// ToolCallError is returned by tools produced by [Tools] when a remote MCP tool
// reports IsError=true. Use [errors.AsType] to distinguish a tool-side
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
	// it (not the prefixed name reported into the registry).
	ToolName string

	// Message is the human-readable failure text reported by the tool,
	// or a fallback when the tool returned IsError=true with no text.
	Message string
}

func (e *ToolCallError) Error() string {
	return fmt.Sprintf("mcp tool %q failed: %s", e.ToolName, e.Message)
}
