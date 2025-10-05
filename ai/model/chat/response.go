package chat

import (
	"errors"
	"time"
)

// Usage tracks token consumption statistics for LLM API requests,
// including both input prompt and generated completion tokens.
type Usage struct {
	PromptTokens     int64       `json:"prompt_tokens"`            // Tokens consumed by input messages
	CompletionTokens int64       `json:"completion_tokens"`        // Tokens generated in response
	OriginalUsage    interface{} `json:"original_usage,omitempty"` // Provider-specific usage data
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

// ensureExtra initializes the extra metadata map if it hasn't been
// created yet to prevent nil pointer operations.
func (r *ResponseMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get retrieves a metadata value by key.
// Returns the value and true if found, or nil and false otherwise.
func (r *ResponseMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	v, ok := r.Extra[key]
	return v, ok
}

// Set stores additional provider-specific metadata with the specified key.
// Automatically initializes the extra map if needed.
func (r *ResponseMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Response represents a complete LLM chat response containing generated
// results and associated metadata.
type Response struct {
	Results  []*Result         `json:"results"`
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse creates a new chat response with results and metadata.
// Returns an error if results are empty or metadata is nil.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("chat response requires at least one result")
	}
	if metadata == nil {
		return nil, errors.New("response metadata is required")
	}
	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

// Result returns the first result from the response for convenient access.
// Returns nil if the response contains no results.
func (c *Response) Result() *Result {
	if len(c.Results) > 0 {
		return c.Results[0]
	}
	return nil
}

// firstToolCallsResult finds and returns the first result containing tool calls.
// Returns nil if no result contains tool/function calls.
func (c *Response) firstToolCallsResult() *Result {
	for _, chatResult := range c.Results {
		if chatResult.AssistantMessage.HasToolCalls() {
			return chatResult
		}
	}
	return nil
}
