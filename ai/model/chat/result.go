package chat

import (
	"errors"
)

// FinishReason indicates why the LLM stopped generating tokens,
// providing context for response completion handling.
type FinishReason string

func (r FinishReason) String() string {
	return string(r)
}

// Standard finish reasons for LLM generation completion
const (
	// FinishReasonStop indicates natural completion or stop sequence reached
	FinishReasonStop FinishReason = "stop"

	// FinishReasonLength indicates truncation due to token limit
	FinishReasonLength FinishReason = "length"

	// FinishReasonToolCalls indicates completion to execute tool/function calls
	FinishReasonToolCalls FinishReason = "tool_calls"

	// FinishReasonContentFilter indicates response blocked by safety filters
	FinishReasonContentFilter FinishReason = "content_filter"

	// FinishReasonReturnDirect indicates direct tool result return without further generation
	FinishReasonReturnDirect FinishReason = "return_direct"

	// FinishReasonOther represents non-standard completion reasons
	FinishReasonOther FinishReason = "other"

	// FinishReasonNull represents undefined or unset finish reason
	FinishReasonNull FinishReason = "null"
)

// ResultMetadata contains completion status and provider-specific metadata
// for a single LLM generation result.
type ResultMetadata struct {
	FinishReason FinishReason   `json:"finish_reason"` // Completion reason
	Extra        map[string]any `json:"extra"`         // Provider-specific metadata
}

// ensureExtra initializes the extra metadata map if it hasn't been
// created yet to prevent nil pointer operations.
func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get retrieves a metadata value by key.
// Returns the value and true if found, or nil and false otherwise.
func (r *ResultMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	v, ok := r.Extra[key]
	return v, ok
}

// Set stores provider-specific metadata with the specified key.
// Automatically initializes the extra map if needed.
func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Result represents a single LLM generation result containing the assistant's
// response, completion metadata, and optional tool execution results.
type Result struct {
	AssistantMessage *AssistantMessage `json:"assistant_message"` // LLM generated response
	Metadata         *ResultMetadata   `json:"metadata"`          // Completion metadata
	ToolMessage      *ToolMessage      `json:"tool_message"`      // Tool execution results (optional)
}

// NewResult creates a new generation result with the required assistant message
// and metadata. Returns an error if either parameter is nil.
func NewResult(assistantMessage *AssistantMessage, metadata *ResultMetadata) (*Result, error) {
	if assistantMessage == nil {
		return nil, errors.New("assistant message is required for result")
	}
	if metadata == nil {
		return nil, errors.New("result metadata is required")
	}

	return &Result{
		AssistantMessage: assistantMessage,
		Metadata:         metadata,
	}, nil
}
