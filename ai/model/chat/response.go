package chat

import (
	"errors"
	"github.com/Tangerg/lynx/ai/model/model"
	"time"
)

// Usage tracks token consumption for LLM API requests.
// Token usage is a critical metric for cost calculation and rate limiting in LLM services.
type Usage struct {
	PromptTokens     int         `json:"prompt_tokens"`            // Tokens consumed by input messages
	CompletionTokens int         `json:"completion_tokens"`        // Tokens generated in LLM response
	OriginalUsage    interface{} `json:"original_usage,omitempty"` // Provider-specific usage data
}

// TotalTokens returns the combined token count for both input and output.
// Used for billing calculations and monitoring LLM API consumption.
func (u *Usage) TotalTokens() int {
	return u.PromptTokens + u.CompletionTokens
}

// RateLimit contains LLM API rate limiting information from the provider.
// Essential for managing API quota and implementing proper backoff strategies.
type RateLimit struct {
	RequestsLimit     int64         `json:"requests_limit"`     // Maximum requests allowed per time window
	RequestsRemaining int64         `json:"requests_remaining"` // Remaining requests in current window
	RequestsReset     time.Duration `json:"requests_reset"`     // Time until request quota resets
	TokensLimit       int64         `json:"tokens_limit"`       // Maximum tokens allowed per time window
	TokensRemaining   int64         `json:"tokens_remaining"`   // Remaining tokens in current window
	TokensReset       time.Duration `json:"tokens_reset"`       // Time until token quota resets
}

// ResponseMetadata contains comprehensive metadata from LLM API responses.
// Includes usage statistics, rate limits, and extensible provider-specific data.
type ResponseMetadata struct {
	model.ResponseMetadata
	ID        string         // Unique identifier for this LLM response
	Model     string         // LLM model name/version used for generation
	Usage     *Usage         // Token consumption details
	RateLimit *RateLimit     // API rate limiting information
	extra     map[string]any // Additional provider-specific metadata
}

// ensureExtra initializes the extra metadata map if not already present.
func (r *ResponseMetadata) ensureExtra() {
	if r.extra == nil {
		r.extra = make(map[string]any)
	}
}

// Extra returns all additional metadata from the LLM provider.
// Useful for accessing provider-specific response information.
func (r *ResponseMetadata) Extra() map[string]any {
	r.ensureExtra()
	return r.extra
}

// Get retrieves a specific metadata value by key.
// Returns the value and a boolean indicating if the key exists.
func (r *ResponseMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	v, ok := r.extra[key]
	return v, ok
}

// Set stores additional metadata from the LLM provider response.
// Allows extending metadata with provider-specific information.
func (r *ResponseMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.extra[key] = value
}

var _ model.Response[*Result, *ResponseMetadata] = (*Response)(nil)

// Response represents a complete LLM chat response with generated content and metadata.
// Contains one or more result variants and comprehensive response metadata.
type Response struct {
	results  []*Result
	metadata *ResponseMetadata
}

// NewResponse creates a new LLM chat response with results and metadata.
// Both parameters are required as they contain essential response information.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if results == nil {
		return nil, errors.New("results is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}
	return &Response{
		results:  results,
		metadata: metadata,
	}, nil
}

// Result returns the primary LLM-generated result.
// Most LLM responses contain a single result, this provides convenient access to it.
func (c *Response) Result() *Result {
	if len(c.results) > 0 {
		return c.results[0]
	}
	return nil
}

// Results returns all LLM-generated result variants.
// Some LLM providers may return multiple response alternatives.
func (c *Response) Results() []*Result {
	return c.results
}

// Metadata returns the comprehensive response metadata from the LLM provider.
// Includes usage statistics, rate limits, and provider-specific information.
func (c *Response) Metadata() *ResponseMetadata {
	return c.metadata
}

// FirstToolCallsResult finds the first LLM result that contains tool/function calls.
// Tool calls enable LLMs to interact with external systems and APIs.
// Returns nil if no result contains tool calls.
func (c *Response) FirstToolCallsResult() *Result {
	for _, chatResult := range c.results {
		if chatResult.Output().HasToolCalls() {
			return chatResult
		}
	}
	return nil
}
