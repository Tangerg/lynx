package messages

import (
	"errors"
	"slices"
)

// ToolReturn represents the response from a tool function execution.
// It contains the result data and identification information that corresponds
// to a specific tool call made by the assistant.
type ToolReturn struct {
	ID     string `json:"id"`     // Unique identifier that matches the original tool call ID
	Name   string `json:"name"`   // Name of the tool function that was executed
	Result string `json:"result"` // The actual response data returned from the tool execution
}

// Compile-time check to ensure ToolMessage implements Message interface.
var _ Message = (*ToolMessage)(nil)

// ToolMessage represents a message containing the results from tool function executions.
// This message type is used to provide the outcomes of function/tool calls back to the
// AI assistant, enabling it to continue the conversation with access to the tool execution
// results. Tool messages typically contain multiple tool returns corresponding to previous
// tool calls made by the assistant.
type ToolMessage struct {
	message
	toolReturns []*ToolReturn // Results from executed tool functions
}

// Type returns the message type as Tool.
func (t *ToolMessage) Type() Type {
	return Tool
}

// ToolReturns returns a slice of tool execution results contained in this message.
func (t *ToolMessage) ToolReturns() []*ToolReturn {
	return t.toolReturns
}

// NewToolMessage creates a new tool message using Go generics for type-safe parameter handling.
// This function provides a flexible API that accepts different parameter types to construct
// tool messages containing execution results from previously called functions.
//
// Supported parameter types:
//   - []*ToolReturn: Sets the tool execution results directly
//   - MessageParams: Complete parameter struct with tool returns and metadata fields
//
// The function uses type constraints and type switching to handle different input types,
// providing a convenient API for creating tool messages with minimal boilerplate code.
//
// Requirements:
//   - At least one tool return must be provided in the toolReturns parameter
//
// Note: Tool messages typically have empty text content since the actual response
// data is contained within the individual tool return results.
//
// Examples:
//
//	NewToolMessage(toolReturnSlice)                       // Creates message with tool returns only
//	NewToolMessage(MessageParams{                         // Creates message with full configuration
//	    ToolReturns: toolReturnSlice,
//	    Metadata: map[string]any{"execution_time": "2ms"},
//	})
//
// Returns:
//   - *ToolMessage: The created tool message containing the execution results
//   - error: Non-nil error if no tool returns are provided
func NewToolMessage[T []*ToolReturn | MessageParams](param T) (*ToolMessage, error) {
	var p MessageParams

	switch typedParam := any(param).(type) {
	case []*ToolReturn:
		p.ToolReturns = typedParam
	case MessageParams:
		p = typedParam
	}

	if len(p.ToolReturns) == 0 {
		return nil, errors.New("tool message must contain at least one tool return")
	}

	return &ToolMessage{
		message:     newMessage("", p.Metadata),
		toolReturns: slices.Clone(p.ToolReturns),
	}, nil
}
