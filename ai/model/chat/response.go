package chat

import (
	"errors"
	"time"
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
	FinishReason FinishReason   `json:"finish_reason"` // Completion reason
	Extra        map[string]any `json:"extra"`         // Provider-specific metadata
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
	AssistantMessage *AssistantMessage `json:"assistant_message"` // LLM generated response
	Metadata         *ResultMetadata   `json:"metadata"`          // Completion metadata
	ToolMessage      *ToolMessage      `json:"tool_message"`      // Tool execution results (optional)
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

// Usage tracks token consumption statistics for LLM API requests,
// including both input prompt and generated completion tokens.
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`            // Tokens consumed by input messages
	CompletionTokens int64 `json:"completion_tokens"`        // Tokens generated in response
	OriginalUsage    any   `json:"original_usage,omitempty"` // Provider-specific usage data
}

// TotalTokens returns the sum of prompt and completion tokens,
// commonly used for cost calculation and quota tracking.
func (u *Usage) TotalTokens() int64 {
	return u.PromptTokens + u.CompletionTokens
}

// RateLimit contains API rate limiting information from the provider,
// including quota limits, remaining quotas, and reset timings.
type RateLimit struct {
	RequestsLimit     int64         `json:"requests_limit"`     // Maximum requests per time window
	RequestsRemaining int64         `json:"requests_remaining"` // Remaining requests in current window
	RequestsReset     time.Duration `json:"requests_reset"`     // Time until request quota resets
	TokensLimit       int64         `json:"tokens_limit"`       // Maximum tokens per time window
	TokensRemaining   int64         `json:"tokens_remaining"`   // Remaining tokens in current window
	TokensReset       time.Duration `json:"tokens_reset"`       // Time until token quota resets
}

// ResponseMetadata contains comprehensive metadata from LLM responses including
// usage statistics, rate limits, and provider-specific attributes.
type ResponseMetadata struct {
	ID        string         `json:"id"`         // Unique response identifier
	Model     string         `json:"model"`      // Model name/version used
	Usage     *Usage         `json:"usage"`      // Token consumption details
	RateLimit *RateLimit     `json:"rate_limit"` // Rate limiting information
	Created   int64          `json:"created"`    // Unix timestamp of response creation
	Extra     map[string]any `json:"extra"`      // Provider-specific metadata
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
	Results  []*Result         `json:"results"`  // Generated results from the LLM
	Metadata *ResponseMetadata `json:"metadata"` // Response metadata
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
