package chat

import (
	"errors"

	"github.com/Tangerg/lynx/core/model"
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

	// FinishReasonToolCalls indicates completion to execute internalTool/function calls
	FinishReasonToolCalls FinishReason = "tool_calls"

	// FinishReasonContentFilter indicates response blocked by safety filters
	FinishReasonContentFilter FinishReason = "content_filter"

	// FinishReasonReturnDirect indicates direct internalTool result return without further generation
	FinishReasonReturnDirect FinishReason = "return_direct"

	// FinishReasonOther represents non-standard completion reasons
	FinishReasonOther FinishReason = "other"

	// FinishReasonNull represents undefined or unset finish reason
	FinishReasonNull FinishReason = "null"
)

// ResultMetadata contains completion status and provider-specific metadata
// for a single LLM generation result.
type ResultMetadata struct {
	// FinishReason Completion reason
	FinishReason FinishReason `json:"finish_reason"`

	// Extra Provider-specific metadata
	Extra map[string]any `json:"extra"`
}

func (m *ResultMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

func (m *ResultMetadata) Get(key string) (any, bool) {
	m.ensureExtra()
	value, exists := m.Extra[key]
	return value, exists
}

func (m *ResultMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Result represents a single LLM generation result containing the assistant's
// response, completion metadata, and optional internalTool execution results.
type Result struct {
	// AssistantMessage LLM generated response
	AssistantMessage *AssistantMessage `json:"assistant_message"`

	// Metadata Completion metadata
	Metadata *ResultMetadata `json:"metadata"`

	// ToolMessage Tool execution results (optional)
	ToolMessage *ToolMessage `json:"tool_message"`
}

// NewResult creates a new generation result with the required assistant message
// and metadata. Returns an error if either parameter is nil.
func NewResult(assistantMessage *AssistantMessage, metadata *ResultMetadata) (*Result, error) {
	if assistantMessage == nil {
		return nil, errors.New("assistant message cannot be nil")
	}
	if metadata == nil {
		return nil, errors.New("result metadata cannot be nil")
	}

	return &Result{
		AssistantMessage: assistantMessage,
		Metadata:         metadata,
	}, nil
}

// Usage is an alias for model.Usage to avoid breaking existing chat code.
type Usage = model.Usage

// RateLimit is an alias for model.RateLimit to avoid breaking existing chat code.
type RateLimit = model.RateLimit

// ResponseMetadata contains comprehensive metadata from LLM responses including
// usage statistics, rate limits, and provider-specific attributes.
type ResponseMetadata struct {
	// ID Unique response identifier
	ID string `json:"id"`

	// Model name/version used
	Model string `json:"model"`

	// Usage Token consumption details
	Usage *Usage `json:"usage"`

	// RateLimit Rate limiting information
	RateLimit *RateLimit `json:"rate_limit"`

	// Created Unix timestamp of response creation
	Created int64 `json:"created"`

	// Extra Provider-specific metadata
	Extra map[string]any `json:"extra"`
}

func (m *ResponseMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

func (m *ResponseMetadata) Get(key string) (any, bool) {
	m.ensureExtra()
	value, exists := m.Extra[key]
	return value, exists
}

func (m *ResponseMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Response represents a complete LLM chat response containing generated
// results and associated metadata.
type Response struct {
	// Results Generated results from the LLM
	Results []*Result `json:"results"`

	// Metadata Response metadata
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse creates a new chat response with results and metadata.
// Returns an error if results are empty or metadata is nil.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("response must contain at least one result")
	}
	if metadata == nil {
		return nil, errors.New("response metadata cannot be nil")
	}

	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

// Result returns the first result from the response for convenient access.
// Returns nil if the response contains no results.
func (r *Response) Result() *Result {
	if len(r.Results) > 0 {
		return r.Results[0]
	}

	return nil
}

// findFirstResultWithToolCalls finds and returns the first result containing internalTool calls.
// Returns nil if no result contains internalTool/function calls.
func (r *Response) findFirstResultWithToolCalls() *Result {
	for _, result := range r.Results {
		if result.AssistantMessage.HasToolCalls() {
			return result
		}
	}

	return nil
}
