package model

import "time"

// Usage tracks token consumption statistics for LLM API requests,
// including both input prompt and generated completion tokens.
//
// Several fields below carry breakdown information that providers may
// surface alongside the headline counts. They are all nullable (*int64)
// to distinguish "provider exposed a 0" from "provider did not report
// this dimension". They are subsets of PromptTokens / CompletionTokens
// and are intentionally NOT added to TotalTokens to avoid double
// counting.
type Usage struct {
	// PromptTokens Token consumed by input messages. Includes any cache
	// read / write portions captured in CacheReadInputTokens /
	// CacheWriteInputTokens.
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
	// absent. ReasoningTokens is a subset of CompletionTokens.
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`

	// CacheReadInputTokens Tokens read from the provider's prompt cache,
	// billed at a reduced rate (Anthropic ephemeral cache, OpenAI cached
	// inputs, etc.). Subset of PromptTokens. Nil means the provider does
	// not support prompt caching or no cache hit occurred for this
	// request.
	CacheReadInputTokens *int64 `json:"cache_read_input_tokens,omitempty"`

	// CacheWriteInputTokens Tokens written to the provider's prompt
	// cache (cache misses that populate the cache for future requests).
	// Typically billed at a premium over standard prompt tokens — for
	// Anthropic ephemeral caches the multiplier depends on TTL. Subset
	// of PromptTokens. Nil means the provider does not support prompt
	// caching or no cache write occurred for this request.
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens,omitempty"`

	// OriginalUsage Provider-specific usage data
	OriginalUsage any `json:"original_usage,omitempty"`
}

// TotalTokens returns the sum of prompt and completion tokens,
// commonly used for cost calculation and quota tracking. The breakdown
// fields (ReasoningTokens, CacheReadInputTokens, CacheWriteInputTokens)
// are intentionally excluded — they are already part of CompletionTokens
// or PromptTokens respectively.
func (u *Usage) TotalTokens() int64 {
	return u.PromptTokens + u.CompletionTokens
}

// HasReasoningTokens reports whether the provider surfaced a breakdown
// of how many completion tokens were spent on reasoning.
func (u *Usage) HasReasoningTokens() bool {
	return u != nil && u.ReasoningTokens != nil
}

// HasCacheReadInputTokens reports whether the provider surfaced a
// prompt-cache hit count for this request.
func (u *Usage) HasCacheReadInputTokens() bool {
	return u != nil && u.CacheReadInputTokens != nil
}

// HasCacheWriteInputTokens reports whether the provider surfaced a
// prompt-cache write count for this request.
func (u *Usage) HasCacheWriteInputTokens() bool {
	return u != nil && u.CacheWriteInputTokens != nil
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
