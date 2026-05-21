package core

import "time"

// LLMInvocation captures the metadata of one LLM call attributed to
// a process. The framework itself never populates these — integration
// code (chat middleware, custom listener) builds the record from the
// provider's response + the configured per-model rate and pushes it
// onto the process via [Process.RecordLLMInvocation]. Subtree
// aggregation then rolls them up through [Process.LLMInvocations].
//
// Cost is in USD by convention but the unit is opaque to the
// framework — callers reporting in other currencies or arbitrary
// budget units are free to do so as long as they stay consistent
// across the process tree.
type LLMInvocation struct {
	// Timestamp is when the call completed. Zero means "now" at the
	// point [Process.RecordLLMInvocation] receives the record.
	Timestamp time.Time

	// Model is the provider-specific identifier (e.g.
	// "claude-sonnet-4-5", "gpt-4o-2024-08-06"). Empty when unknown.
	Model string

	// Provider is the lynx provider id ("anthropic", "openai", ...).
	// Empty when unknown.
	Provider string

	// Cost is the dollar amount the provider charged. Zero means
	// either "no cost reported" or "explicitly free" — disambiguate
	// at the integration layer when needed.
	Cost float64

	// PromptTokens / CompletionTokens / ReasoningTokens mirror the
	// shape of [chat.Usage]. ReasoningTokens is the chain-of-thought
	// subset of CompletionTokens (already counted there) — kept
	// separate so callers can attribute reasoning spend.
	PromptTokens     int64
	CompletionTokens int64
	ReasoningTokens  int64

	// CacheReadInputTokens / CacheWriteInputTokens carry prompt-cache
	// attribution (Anthropic prompt caching, OpenAI cached inputs).
	// Both are subsets of PromptTokens.
	CacheReadInputTokens  int64
	CacheWriteInputTokens int64

	// Duration is the wall-clock time the call took. Zero means
	// "unknown / not measured".
	Duration time.Duration

	// Action is the action name that issued the call, when known.
	// Empty for calls made outside an action (e.g. a top-level
	// PromptRunner invocation from outside the runtime).
	Action string
}

// EmbeddingInvocation captures one embedding call attributed to a
// process. Mirrors [LLMInvocation] minus completion / reasoning
// fields that don't apply to embeddings.
type EmbeddingInvocation struct {
	Timestamp time.Time
	Model     string
	Provider  string
	Cost      float64

	// InputTokens is the prompt-side token count (embeddings don't
	// produce completion tokens).
	InputTokens int64

	// InputCount is the number of texts embedded in this call. Some
	// providers charge per-text + per-token; carry both so cost
	// allocators have the data they need.
	InputCount int

	Duration time.Duration
	Action   string
}

// TokenTotals is the aggregated view of an invocation slice. The
// framework computes these as a convenience for budget policies and
// UI layers that want one number per dimension.
type TokenTotals struct {
	PromptTokens          int64
	CompletionTokens      int64
	ReasoningTokens       int64
	CacheReadInputTokens  int64
	CacheWriteInputTokens int64
	InputTokens           int64 // embeddings
}
