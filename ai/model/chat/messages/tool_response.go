package messages

import (
	"errors"
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

// NewToolResponseMessage creates a new tool response message with the given toolResponses.
//
// The toolResponses parameter must contain at least one response.
//
// Optionally accepts metadata as a map. If multiple metadata maps are provided,
// only the first one will be used.
//
// Note: ToolResponseMessage typically has empty text content as the actual content
// is contained within the tool toolResponses.
func NewToolResponseMessage(toolResponses []*ToolResponse, metadata ...map[string]any) (*ToolResponseMessage, error) {
	if len(toolResponses) == 0 {
		return nil, errors.New("tool response message must contain at least one tool response")
	}
	return &ToolResponseMessage{
		message:       newmessage(Tool, "", metadata...), //toolcall not have text content
		toolResponses: toolResponses,
	}, nil
}
