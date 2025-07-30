package messages

import (
	"errors"
	"slices"
)

// ToolResponse represents the response from a tool call execution.
// It contains the result data and identification information for the tool call.
type ToolResponse struct {
	ID           string `json:"id"`            // Unique identifier matching the original tool call
	Name         string `json:"name"`          // Name of the tool that was called
	ResponseData string `json:"response_data"` // The actual response data from the tool execution
}

// Compile-time check to ensure ToolResponseMessage implements Message interface.
var _ Message = (*ToolResponseMessage)(nil)

// ToolResponseMessage represents a message containing toolResponses from tool calls.
// This message type is used to provide the results of function/tool executions
// back to the AI assistant, allowing it to continue the conversation with
// the tool execution results.
type ToolResponseMessage struct {
	message
	toolResponses []*ToolResponse
}

// ToolResponses returns the tool toolResponses contained in this message.
func (t *ToolResponseMessage) ToolResponses() []*ToolResponse {
	return t.toolResponses
}

type ToolResponseMessageParam struct {
	ToolResponses []*ToolResponse
	Metadata      map[string]any
}

// NewToolResponseMessage creates a new tool response message using Go generics to simulate function overloading.
// This allows creating tool response messages with different parameter types in a single function call.
//
// Supported parameter types:
//   - []*ToolResponse: Sets the tool responses directly
//   - ToolResponseMessageParam: Complete parameter struct with tool responses and metadata fields
//
// The function uses type constraints and type switching to handle different input types,
// providing a convenient API that mimics function overloading found in other languages.
//
// Requirements:
//   - The toolResponses parameter must contain at least one response
//
// Note: ToolResponseMessage typically has empty text content as the actual content
// is contained within the tool responses.
//
// Examples:
//
//	NewToolResponseMessage(toolResponseSlice)           // Tool responses only
//	NewToolResponseMessage(ToolResponseMessageParam{...}) // Full configuration
//
// Returns:
//   - *ToolResponseMessage: The created message
//   - error: Non-nil if no tool responses are provided
func NewToolResponseMessage[T []*ToolResponse | ToolResponseMessageParam](param T) (*ToolResponseMessage, error) {
	var p ToolResponseMessageParam

	switch typedParam := any(param).(type) {
	case []*ToolResponse:
		p.ToolResponses = typedParam
	case ToolResponseMessageParam:
		p = typedParam
	}
	if len(p.ToolResponses) == 0 {
		return nil, errors.New("tool response message must contain at least one tool response")
	}
	return &ToolResponseMessage{
		message:       newMessage(Tool, "", p.Metadata), //toolcall not have text content
		toolResponses: slices.Clone(p.ToolResponses),
	}, nil
}
