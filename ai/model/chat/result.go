package chat

import (
	"errors"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/model"
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
	// Stop indicates the LLM completed generation naturally or hit a stop sequence
	Stop FinishReason = "stop"

	// Length indicates the LLM response was truncated due to token limits
	Length FinishReason = "length"

	// ToolCalls indicates the LLM finished to execute function/tool calls
	ToolCalls FinishReason = "tool_calls"

	// ContentFilter indicates the LLM response was blocked by safety filters
	ContentFilter FinishReason = "content_filter"

	// ReturnDirect indicates tool results were returned without further LLM processing
	// Occurs when all executed tools have ReturnDirect=true
	ReturnDirect FinishReason = "return_direct"

	// Other represents any LLM completion reason not covered by standard cases
	Other FinishReason = "other"

	// Null represents an undefined or unset finish reason
	Null FinishReason = "null"
)

// IsStop returns true if the LLM completed generation naturally.
func (r FinishReason) IsStop() bool {
	return r == Stop
}

// IsLength returns true if the LLM response was truncated due to token limits.
func (r FinishReason) IsLength() bool {
	return r == Length
}

// IsToolCalls returns true if the LLM requested tool/function execution.
func (r FinishReason) IsToolCalls() bool {
	return r == ToolCalls
}

// IsContentFilter returns true if the LLM response was blocked by safety filters.
func (r FinishReason) IsContentFilter() bool {
	return r == ContentFilter
}

// IsReturnDirect returns true if tool results bypassed further LLM processing.
func (r FinishReason) IsReturnDirect() bool {
	return r == ReturnDirect
}

// IsOther returns true if the LLM completion reason is non-standard.
func (r FinishReason) IsOther() bool {
	return r == Other
}

// IsNull returns true if the finish reason is undefined.
func (r FinishReason) IsNull() bool {
	return r == Null
}

// ResultMetadata contains metadata for a single LLM generation result.
// Includes completion status and extensible provider-specific information.
type ResultMetadata struct {
	model.ResultMetadata
	FinishReason FinishReason   // Why the LLM stopped generating
	extra        map[string]any // Additional provider-specific metadata
}

// ensureExtra initializes the extra metadata map if not present.
func (r *ResultMetadata) ensureExtra() {
	if r.extra == nil {
		r.extra = make(map[string]any)
	}
}

// Extra returns all additional metadata from the LLM provider.
func (r *ResultMetadata) Extra() map[string]any {
	r.ensureExtra()
	return r.extra
}

// Get retrieves a specific metadata value by key.
func (r *ResultMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	v, ok := r.extra[key]
	return v, ok
}

// Set stores additional LLM provider-specific metadata.
func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.extra[key] = value
}

var _ model.Result[*messages.AssistantMessage, *ResultMetadata] = (*Result)(nil)

// Result represents a single LLM generation result with associated metadata.
// Contains the LLM's response and information about how the generation completed.
//
// Supports both standard LLM conversations and tool-enhanced workflows:
// - Standard: Direct LLM text response with completion metadata
// - Tool-enhanced: LLM response with tool calls and optional tool execution results
type Result struct {
	assistantMessage    *messages.AssistantMessage    // LLM's generated response (required)
	metadata            *ResultMetadata               // Generation metadata (required)
	toolResponseMessage *messages.ToolResponseMessage // Optional tool execution results
}

// ToolResponseMessage returns tool execution results if available.
// Used in tool-enhanced LLM workflows where the LLM can call external functions.
func (r *Result) ToolResponseMessage() *messages.ToolResponseMessage {
	return r.toolResponseMessage
}

// Output returns the LLM's generated response message.
// May contain text content, tool calls, or both depending on the LLM's decision.
func (r *Result) Output() *messages.AssistantMessage {
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
//   - toolResponseMessage: Optional tool execution results
//
// Returns an error if required parameters are missing.
func NewResult(assistantMessage *messages.AssistantMessage, metadata *ResultMetadata, toolResponseMessage ...*messages.ToolResponseMessage) (*Result, error) {
	if assistantMessage == nil {
		return nil, errors.New("assistant message is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}

	return &Result{
		assistantMessage:    assistantMessage,
		metadata:            metadata,
		toolResponseMessage: pkgSlices.FirstOr(toolResponseMessage, nil),
	}, nil
}
