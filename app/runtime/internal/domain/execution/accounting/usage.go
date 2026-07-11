// Package accounting holds token and cost accounting value objects shared by
// turn execution, delivery, and pricing adapters.
package accounting

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

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
func (t TokenUsage) Total() int64 {
	return t.PromptTokens + t.CompletionTokens
}

// AddInvocation folds one model invocation's token counts into this roll-up.
func (t *TokenUsage) AddInvocation(inv core.LLMInvocation) {
	t.PromptTokens += inv.PromptTokens
	t.CompletionTokens += inv.CompletionTokens
	t.ReasoningTokens += inv.ReasoningTokens
	t.CacheReadTokens += inv.CacheReadInputTokens
	t.CacheWriteTokens += inv.CacheWriteInputTokens
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
