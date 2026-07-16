package core

import (
	"math"
	"time"
)

// ModelCall captures the metadata of one LLM call attributed to
// a process. The framework itself never populates these — integration
// code (chat middleware, custom listener) builds the record from the
// provider's response + the configured per-model rate and pushes it
// onto the process via [UsageRecorder.RecordModelCall]. Subtree
// aggregation then rolls them up through [ProcessView.ModelCalls].
//
// CostUSD is in USD by convention but the unit is opaque to the
// framework — callers reporting in other currencies or arbitrary
// budget units are free to do so as long as they stay consistent
// across the process tree.
type ModelCall struct {
	// Timestamp is when the call completed. Zero means "now" at the
	// point [UsageRecorder.RecordModelCall] receives the record.
	Timestamp time.Time `json:"timestamp"`

	// Model is the provider-specific identifier (e.g.
	// "claude-sonnet-4-5", "gpt-4o-2024-08-06"). Empty when unknown.
	Model string `json:"model"`

	// Provider is the provider id ("anthropic", "openai", ...).
	// Empty when unknown.
	Provider string `json:"provider,omitempty"`

	// CostUSD is the dollar amount the provider charged. Zero means
	// either "no cost reported" or "explicitly free" — disambiguate
	// at the integration layer when needed.
	CostUSD float64 `json:"cost_usd"`

	// PromptTokens / CompletionTokens / ReasoningTokens mirror the
	// shape of [chat.Usage]. ReasoningTokens is the chain-of-thought
	// subset of CompletionTokens (already counted there) — kept
	// separate so callers can attribute reasoning spend.
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty"`

	// CacheReadInputTokens / CacheWriteInputTokens carry prompt-cache
	// attribution (Anthropic prompt caching, OpenAI cached inputs).
	// Both are subsets of PromptTokens.
	CacheReadInputTokens  int64 `json:"cache_read_input_tokens,omitempty"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens,omitempty"`

	// Duration is the wall-clock time the call took. Zero means
	// "unknown / not measured".
	Duration time.Duration `json:"duration_ns"`

	// ActionName identifies the action that issued the call, when known.
	// Empty for calls made outside an action (e.g. a top-level
	// Prompt invocation from outside the runtime).
	ActionName string `json:"action,omitempty"`
}

// EmbeddingCall captures one embedding call attributed to a
// process. Mirrors [ModelCall] minus completion / reasoning
// fields that don't apply to embeddings.
type EmbeddingCall struct {
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider,omitempty"`
	CostUSD   float64   `json:"cost_usd"`

	// InputTokens is the prompt-side token count (embeddings don't
	// produce completion tokens).
	InputTokens int64 `json:"input_tokens"`

	// InputCount is the number of texts embedded in this call. Some
	// providers charge per-text + per-token; carry both so cost
	// allocators have the data they need.
	InputCount int `json:"input_count"`

	Duration   time.Duration `json:"duration_ns"`
	ActionName string        `json:"action,omitempty"`
}

func (c ModelCall) valid() bool {
	return !c.Timestamp.IsZero() &&
		!math.IsNaN(c.CostUSD) && !math.IsInf(c.CostUSD, 0) && c.CostUSD >= 0 &&
		c.PromptTokens >= 0 && c.CompletionTokens >= 0 && c.ReasoningTokens >= 0 &&
		c.CacheReadInputTokens >= 0 && c.CacheWriteInputTokens >= 0 &&
		c.ReasoningTokens <= c.CompletionTokens && c.Duration >= 0
}

func (c EmbeddingCall) valid() bool {
	return !c.Timestamp.IsZero() &&
		!math.IsNaN(c.CostUSD) && !math.IsInf(c.CostUSD, 0) && c.CostUSD >= 0 &&
		c.InputTokens >= 0 && c.InputCount >= 0 && c.Duration >= 0
}
