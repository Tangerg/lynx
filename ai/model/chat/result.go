package chat

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// FinishReason indicates why the LLM stopped generating tokens.
// Understanding finish reasons is crucial for handling different LLM completion scenarios.
type FinishReason string

func (r FinishReason) String() string {
	return string(r)
}

// LLM completion finish reasons
const (
	// FinishReasonStop indicates the LLM completed generation naturally or hit a stop sequence
	FinishReasonStop FinishReason = "stop"

	// FinishReasonLength indicates the LLM response was truncated due to token limits
	FinishReasonLength FinishReason = "length"

	// FinishReasonToolCalls indicates the LLM finished to call function/tool calls
	FinishReasonToolCalls FinishReason = "tool_calls"

	// FinishReasonContentFilter indicates the LLM response was blocked by safety filters
	FinishReasonContentFilter FinishReason = "content_filter"

	// FinishReasonReturnDirect indicates tool results were returned without further LLM processing
	// Occurs when all executed tools have FinishReasonReturnDirect=true
	FinishReasonReturnDirect FinishReason = "return_direct"

	// FinishReasonOther represents any LLM completion reason not covered by standard cases
	FinishReasonOther FinishReason = "other"

	// FinishReasonNull represents an undefined or unset finish reason
	FinishReasonNull FinishReason = "null"
)

// IsStop returns true if the LLM completed generation naturally.
func (r FinishReason) IsStop() bool {
	return r == FinishReasonStop
}

// IsLength returns true if the LLM response was truncated due to token limits.
func (r FinishReason) IsLength() bool {
	return r == FinishReasonLength
}

// IsToolCalls returns true if the LLM requested tool/function execution.
func (r FinishReason) IsToolCalls() bool {
	return r == FinishReasonToolCalls
}

// IsContentFilter returns true if the LLM response was blocked by safety filters.
func (r FinishReason) IsContentFilter() bool {
	return r == FinishReasonContentFilter
}

// IsReturnDirect returns true if tool results bypassed further LLM processing.
func (r FinishReason) IsReturnDirect() bool {
	return r == FinishReasonReturnDirect
}

// IsOther returns true if the LLM completion reason is non-standard.
func (r FinishReason) IsOther() bool {
	return r == FinishReasonOther
}

// IsNull returns true if the finish reason is undefined.
func (r FinishReason) IsNull() bool {
	return r == FinishReasonNull
}

var _ model.ResultMetadata = (*ResultMetadata)(nil)

// ResultMetadata contains metadata for a single LLM generation result.
// Includes completion status and extensible provider-specific information.
type ResultMetadata struct {
	FinishReason FinishReason   // Why the LLM stopped generating
	Extra        map[string]any // Additional provider-specific metadata
}

// ensureExtra initializes the extra metadata map if not present.
func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get retrieves a specific metadata value by key.
func (r *ResultMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	v, ok := r.Extra[key]
	return v, ok
}

// Set stores additional LLM provider-specific metadata.
func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

var _ model.Result[*AssistantMessage, *ResultMetadata] = (*Result)(nil)

// Result represents a single LLM generation result with associated metadata.
// Contains the LLM's response and information about how the generation completed.
//
// Supports both standard LLM conversations and tool-enhanced workflows:
// - Standard: Direct LLM text response with completion metadata
// - MessageTypeTool-enhanced: LLM response with tool calls and optional tool execution results
type Result struct {
	assistantMessage *AssistantMessage // LLM's generated response (required)
	metadata         *ResultMetadata   // Generation metadata (required)
	toolMessage      *ToolMessage      // Optional tool execution results
}

// ToolMessage returns tool execution results if available.
// Used in tool-enhanced LLM workflows where the LLM can call external functions.
func (r *Result) ToolMessage() *ToolMessage {
	return r.toolMessage
}

// Output returns the LLM's generated response message.
// May contain text content, tool calls, or both depending on the LLM's decision.
func (r *Result) Output() *AssistantMessage {
	return r.assistantMessage
}

// Metadata returns generation metadata including finish reason and provider details.
// Essential for understanding how and why the LLM completed generation.
func (r *Result) Metadata() *ResultMetadata {
	return r.metadata
}

// NewResult creates a new LLM generation result with required components.
// Both assistant message and metadata are mandatory for proper result handling.
//
// Parameters:
//   - assistantMessage: The LLM's generated response (required)
//   - metadata: Generation completion metadata (required)
//   - toolMessage: Optional tool execution results
//
// Returns an error if required parameters are missing.
func NewResult(assistantMessage *AssistantMessage, metadata *ResultMetadata, toolMessages ...*ToolMessage) (*Result, error) {
	if assistantMessage == nil {
		return nil, errors.New("assistant message is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}

	return &Result{
		assistantMessage: assistantMessage,
		metadata:         metadata,
		toolMessage:      pkgSlices.FirstOr(toolMessages, nil),
	}, nil
}
