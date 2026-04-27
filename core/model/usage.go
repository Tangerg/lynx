package model

import "time"

// Usage tracks token consumption statistics for LLM API requests,
// including both input prompt and generated completion tokens.
type Usage struct {
	// PromptTokens Token consumed by input messages
	PromptTokens int64 `json:"prompt_tokens"`

	// CompletionTokens Token generated in response. For reasoning models
	// (OpenAI o-series, DeepSeek-R1, Claude extended thinking), this
	// already includes any tokens billed for the chain-of-thought; the
	// ReasoningTokens field below carries the breakdown subset.
	CompletionTokens int64 `json:"completion_tokens"`

	// ReasoningTokens Tokens consumed specifically by the model's
	// reasoning / chain-of-thought process, when the provider exposes
	// this breakdown. Mirrors OpenAI's
	// completion_tokens_details.reasoning_tokens. Nil means the provider
	// did not surface a breakdown — it is NOT a hint that reasoning was
	// absent. ReasoningTokens is a subset of CompletionTokens and is NOT
	// added to TotalTokens to avoid double counting.
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`

	// OriginalUsage Provider-specific usage data
	OriginalUsage any `json:"original_usage,omitempty"`
}

// TotalTokens returns the sum of prompt and completion tokens,
// commonly used for cost calculation and quota tracking. ReasoningTokens
// is intentionally excluded — it is already part of CompletionTokens.
func (u *Usage) TotalTokens() int64 {
	return u.PromptTokens + u.CompletionTokens
}

// HasReasoningTokens reports whether the provider surfaced a breakdown
// of how many completion tokens were spent on reasoning.
func (u *Usage) HasReasoningTokens() bool {
	return u != nil && u.ReasoningTokens != nil
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
