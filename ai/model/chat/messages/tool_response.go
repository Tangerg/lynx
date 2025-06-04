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

// ToolResponseMessage represents a message containing responses from tool calls.
// This message type is used to provide the results of function/tool executions
// back to the AI assistant, allowing it to continue the conversation with
// the tool execution results.
type ToolResponseMessage struct {
	message
	responses []*ToolResponse // List of tool responses
}

// Responses returns the tool responses contained in this message.
func (t *ToolResponseMessage) Responses() []*ToolResponse {
	return t.responses
}

// NewToolResponseMessage creates a new tool response message with the given responses.
//
// The responses parameter must contain at least one response.
//
// Optionally accepts metadata as a map. If multiple metadata maps are provided,
// only the first one will be used.
//
// Note: ToolResponseMessage typically has empty text content as the actual content
// is contained within the tool responses.
func NewToolResponseMessage(responses []*ToolResponse, metadata ...map[string]any) (*ToolResponseMessage, error) {
	if len(responses) == 0 {
		return nil, errors.New("tool responses must contain at least one response")
	}
	return &ToolResponseMessage{
		message:   newmessage(Tool, "", metadata...), //toolcall not have text content
		responses: responses,
	}, nil
}
