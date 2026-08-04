// Package accounting holds token and cost accounting value objects for model
// execution and pricing.
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

// Add folds another model-call roll-up into this one for cumulative execution
// totals and per-model breakdowns.
func (t *TokenUsage) Add(u TokenUsage) {
	t.PromptTokens += u.PromptTokens
	t.CompletionTokens += u.CompletionTokens
	t.ReasoningTokens += u.ReasoningTokens
	t.CacheReadTokens += u.CacheReadTokens
	t.CacheWriteTokens += u.CacheWriteTokens
}

// ModelUsage is one model's slice of an execution's tokens and cost.
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

// Total returns the checked aggregate of every model in the snapshot. The
// result intentionally has an empty Model because it represents the whole
// process subtree rather than another served model.
func (s Snapshot) Total() (ModelUsage, error) {
	if err := s.Validate(); err != nil {
		return ModelUsage{}, err
	}
	var total ModelUsage
	for index, model := range s.Models {
		var ok bool
		if total.PromptTokens, ok = checkedAddInt64(total.PromptTokens, model.PromptTokens); !ok {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] prompt-token aggregate overflows", index)
		}
		if total.CompletionTokens, ok = checkedAddInt64(total.CompletionTokens, model.CompletionTokens); !ok {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] completion-token aggregate overflows", index)
		}
		if total.ReasoningTokens, ok = checkedAddInt64(total.ReasoningTokens, model.ReasoningTokens); !ok {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] reasoning-token aggregate overflows", index)
		}
		if total.CacheReadTokens, ok = checkedAddInt64(total.CacheReadTokens, model.CacheReadTokens); !ok {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] cache-read-token aggregate overflows", index)
		}
		if total.CacheWriteTokens, ok = checkedAddInt64(total.CacheWriteTokens, model.CacheWriteTokens); !ok {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] cache-write-token aggregate overflows", index)
		}
		if model.CostUSD > math.MaxFloat64-total.CostUSD {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] cost aggregate overflows", index)
		}
		if model.Calls > math.MaxInt-total.Calls {
			return ModelUsage{}, fmt.Errorf("accounting snapshot: models[%d] call aggregate overflows", index)
		}
		total.CostUSD += model.CostUSD
		total.Calls += model.Calls
	}
	return total, nil
}

func checkedAddInt64(left, right int64) (int64, bool) {
	if right < 0 || left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
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

// ValidateAdvanceFrom proves that s is a cumulative continuation of previous.
// A checkpoint may add models or increase counters, but it cannot erase a model
// or rewind usage already committed at an earlier barrier.
func (s Snapshot) ValidateAdvanceFrom(previous Snapshot) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous usage: %w", err)
	}
	if err := s.Validate(); err != nil {
		return fmt.Errorf("next usage: %w", err)
	}
	nextByModel := make(map[string]ModelUsage, len(s.Models))
	for _, model := range s.Models {
		nextByModel[model.Model] = model
	}
	for _, before := range previous.Models {
		after, found := nextByModel[before.Model]
		if !found {
			return fmt.Errorf("accounting snapshot: model %q disappeared", before.Model)
		}
		if after.PromptTokens < before.PromptTokens ||
			after.CompletionTokens < before.CompletionTokens ||
			after.ReasoningTokens < before.ReasoningTokens ||
			after.CacheReadTokens < before.CacheReadTokens ||
			after.CacheWriteTokens < before.CacheWriteTokens ||
			after.CostUSD < before.CostUSD ||
			after.Calls < before.Calls {
			return fmt.Errorf("accounting snapshot: model %q usage regressed", before.Model)
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

// Pricing computes the USD cost of one LLM round from the provider, served
// model, and full token usage.
type Pricing func(provider, model string, usage *chat.Usage) float64
