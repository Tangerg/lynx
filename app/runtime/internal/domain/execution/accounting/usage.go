// Package accounting holds token and cost accounting value objects shared by
// turn execution, delivery, and pricing adapters.
package accounting

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
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
	Calls   int
}

// Snapshot is the durable, application-owned usage projection for one complete
// process tree. Models are unique and sorted by model ID so concurrent
// execution cannot make checkpoint bytes or output ordering nondeterministic.
type Snapshot struct {
	Models []ModelUsage
}

// Validate checks that a persisted usage projection is canonical and safe to
// aggregate.
func (s Snapshot) Validate() error {
	var previous string
	for index, model := range s.Models {
		if strings.TrimSpace(model.Model) == "" || model.Model != strings.TrimSpace(model.Model) {
			return fmt.Errorf("accounting snapshot: models[%d] has invalid model", index)
		}
		if index > 0 && model.Model <= previous {
			return errors.New("accounting snapshot: models must be unique and sorted by model ID")
		}
		previous = model.Model
		if err := model.Validate(); err != nil {
			return fmt.Errorf("accounting snapshot: models[%d]: %w", index, err)
		}
	}
	return nil
}

// Validate checks one model's token and cost counters.
func (m ModelUsage) Validate() error {
	u := m.TokenUsage
	if u.PromptTokens < 0 || u.CompletionTokens < 0 || u.ReasoningTokens < 0 ||
		u.CacheReadTokens < 0 || u.CacheWriteTokens < 0 {
		return errors.New("model usage token counts must not be negative")
	}
	if u.ReasoningTokens > u.CompletionTokens {
		return errors.New("model usage reasoning tokens exceed completion tokens")
	}
	if u.CacheReadTokens > u.PromptTokens || u.CacheWriteTokens > u.PromptTokens {
		return errors.New("model usage cache tokens exceed prompt tokens")
	}
	if math.IsNaN(m.CostUSD) || math.IsInf(m.CostUSD, 0) || m.CostUSD < 0 {
		return errors.New("model usage cost must be finite and non-negative")
	}
	if m.Calls <= 0 {
		return errors.New("model usage calls must be positive")
	}
	return nil
}

// Budget caps one turn by tokens, cost, and model calls across its complete
// delegation tree. A zero field is unbounded on that dimension.
type Budget struct {
	MaxTokens  int64
	MaxCostUSD float64
	MaxSteps   int
}

// Pricing computes the USD cost of one LLM round from the provider, served
// model, and full token usage.
type Pricing func(provider, model string, usage *chat.Usage) float64
