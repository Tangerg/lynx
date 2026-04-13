package model

import "time"

// Usage tracks token consumption statistics for LLM API requests,
// including both input prompt and generated completion tokens.
type Usage struct {
	// PromptTokens Token consumed by input messages
	PromptTokens int64 `json:"prompt_tokens"`

	// CompletionTokens Token generated in response
	CompletionTokens int64 `json:"completion_tokens"`

	// OriginalUsage Provider-specific usage data
	OriginalUsage any `json:"original_usage,omitempty"`
}

// TotalTokens returns the sum of prompt and completion tokens,
// commonly used for cost calculation and quota tracking.
func (u *Usage) TotalTokens() int64 {
	return u.PromptTokens + u.CompletionTokens
}

// RateLimit contains API rate limiting information from the provider,
// including quota limits, remaining quotas, and reset timings.
type RateLimit struct {
	// RequestsLimit Maximum requests per time window
	RequestsLimit int64 `json:"requests_limit"`

	// RequestsRemaining Remaining requests in current window
	RequestsRemaining int64 `json:"requests_remaining"`

	// RequestsReset Time until request quota resets
	RequestsReset time.Duration `json:"requests_reset"`

	// TokensLimit Maximum tokens per time window
	TokensLimit int64 `json:"tokens_limit"`

	// TokensRemaining Remaining tokens in current window
	TokensRemaining int64 `json:"tokens_remaining"`

	// TokensReset Time until token quota resets
	TokensReset time.Duration `json:"tokens_reset"`
}
