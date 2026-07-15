// Package accounting holds token and cost accounting value objects shared by
// turn execution, delivery, and pricing adapters.
package accounting

import "github.com/Tangerg/lynx/core/chat"

// TokenUsage is a token roll-up. ReasoningTokens is the chain-of-thought
// subset of CompletionTokens, so total counts only prompt + completion.
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// Total is prompt + completion: the figure a token budget caps.
func (t *TokenUsage) Total() int64 {
	return t.PromptTokens + t.CompletionTokens
}

// Add folds another token roll-up into this one — used to accumulate per-round
// usage into a turn total + per-model breakdown. The caller (the agent-execution
// adapter, which owns the SDK invocation type) maps a model round to a
// [TokenUsage], keeping this domain value free of the agent SDK.
func (t *TokenUsage) Add(u TokenUsage) {
	t.PromptTokens += u.PromptTokens
	t.CompletionTokens += u.CompletionTokens
	t.ReasoningTokens += u.ReasoningTokens
	t.CacheReadTokens += u.CacheReadTokens
	t.CacheWriteTokens += u.CacheWriteTokens
}

// ModelUsage is one model's slice of a turn's tokens and cost.
type ModelUsage struct {
	Model string
	TokenUsage
	CostUSD float64
}

// Budget caps one turn by tokens, cost, and tool-call rounds. A zero field is
// unbounded on that dimension.
type Budget struct {
	MaxTokens  int64
	MaxCostUSD float64
	MaxSteps   int
}

// UsageExceeded reports whether cumulative token or cost usage reached its
// configured limit.
func (b Budget) UsageExceeded(tokens int64, costUSD float64) bool {
	return (b.MaxTokens > 0 && tokens >= b.MaxTokens) ||
		(b.MaxCostUSD > 0 && costUSD >= b.MaxCostUSD)
}

// StepsExceeded reports whether completed tool-call rounds reached their
// configured limit.
func (b Budget) StepsExceeded(steps int) bool {
	return b.MaxSteps > 0 && steps >= b.MaxSteps
}

// Pricing computes the USD cost of one LLM round from the provider, served
// model, and full token usage.
type Pricing func(provider, model string, usage *chat.Usage) float64
