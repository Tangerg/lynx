// Package accounting holds the token and cost accounting value objects shared
// by the kernel and turn event layer.
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

// ModelUsage is one model's slice of a turn's tokens + cost.
type ModelUsage struct {
	Model string
	TokenUsage
	CostUSD float64
}

// Pricing computes the USD cost of one LLM round from the provider, served
// model, and full token usage.
type Pricing func(provider, model string, usage *chat.Usage) float64
