// Package accounting holds token and cost accounting value objects for model
// execution and pricing.
package accounting

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/chat"
)

// Totals is the cumulative token and cost fact reported for one scope. CostUSD
// is absent when pricing was not available; an absent price is intentionally
// distinct from a reported zero price.
type Totals struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

// Clone returns an ownership-isolated value.
func (t Totals) Clone() Totals {
	if t.CostUSD != nil {
		cost := *t.CostUSD
		t.CostUSD = &cost
	}
	return t
}

// Validate reports whether the cumulative counters are internally consistent.
func (t Totals) Validate() error {
	if t.InputTokens < 0 || t.OutputTokens < 0 || t.CacheReadTokens < 0 ||
		t.CacheWriteTokens < 0 || t.ReasoningTokens < 0 {
		return errors.New("accounting: token counts must not be negative")
	}
	if t.CostUSD != nil && (*t.CostUSD < 0 || math.IsNaN(*t.CostUSD) || math.IsInf(*t.CostUSD, 0)) {
		return errors.New("accounting: cost must be finite and non-negative")
	}
	return nil
}

// ValidateAdvanceFrom proves that t has not erased previously committed
// cumulative accounting.
func (t Totals) ValidateAdvanceFrom(previous Totals) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous totals: %w", err)
	}
	if err := t.Validate(); err != nil {
		return fmt.Errorf("next totals: %w", err)
	}
	if t.InputTokens < previous.InputTokens || t.OutputTokens < previous.OutputTokens ||
		t.CacheReadTokens < previous.CacheReadTokens || t.CacheWriteTokens < previous.CacheWriteTokens ||
		t.ReasoningTokens < previous.ReasoningTokens || t.cost() < previous.cost() {
		return errors.New("accounting: cumulative totals regressed")
	}
	return nil
}

func (t Totals) cost() float64 {
	if t.CostUSD == nil {
		return 0
	}
	return *t.CostUSD
}

// Equal reports semantic equality, preserving the distinction between absent
// and reported-zero pricing.
func (t Totals) Equal(other Totals) bool {
	if t.InputTokens != other.InputTokens || t.OutputTokens != other.OutputTokens ||
		t.CacheReadTokens != other.CacheReadTokens || t.CacheWriteTokens != other.CacheWriteTokens ||
		t.ReasoningTokens != other.ReasoningTokens {
		return false
	}
	if t.CostUSD == nil || other.CostUSD == nil {
		return t.CostUSD == nil && other.CostUSD == nil
	}
	return *t.CostUSD == *other.CostUSD
}

// Usage is cumulative accounting for a Run. Total remains authoritative when
// a provider cannot attribute usage to individual models; ByModel is the
// optional breakdown and never replaces the total.
type Usage struct {
	Total   Totals
	ByModel map[string]Totals
}

// Clone returns an ownership-isolated usage value.
func (u Usage) Clone() Usage {
	u.Total = u.Total.Clone()
	if u.ByModel != nil {
		source := u.ByModel
		u.ByModel = make(map[string]Totals, len(source))
		for model, totals := range source {
			u.ByModel[model] = totals.Clone()
		}
	}
	return u
}

// Validate reports whether u is safe to persist and compare.
func (u Usage) Validate() error {
	if err := u.Total.Validate(); err != nil {
		return fmt.Errorf("accounting: total usage: %w", err)
	}
	models := make([]string, 0, len(u.ByModel))
	for model := range u.ByModel {
		models = append(models, model)
	}
	slices.Sort(models)
	for _, model := range models {
		if strings.TrimSpace(model) == "" || model != strings.TrimSpace(model) {
			return errors.New("accounting: model identity is required without surrounding whitespace")
		}
		if err := u.ByModel[model].Validate(); err != nil {
			return fmt.Errorf("accounting: model %q: %w", model, err)
		}
	}
	return nil
}

// ValidateAdvanceFrom proves that u is a cumulative continuation of
// previous. Once a provider reports usage or a per-model key, it cannot vanish.
func (u Usage) ValidateAdvanceFrom(previous Usage) error {
	if err := u.Validate(); err != nil {
		return err
	}
	if err := u.Total.ValidateAdvanceFrom(previous.Total); err != nil {
		return err
	}
	for model, before := range previous.ByModel {
		after, found := u.ByModel[model]
		if !found {
			return fmt.Errorf("accounting: cumulative usage dropped model %q", model)
		}
		if err := after.ValidateAdvanceFrom(before); err != nil {
			return fmt.Errorf("accounting: model %q: %w", model, err)
		}
	}
	return nil
}

// Equal reports whether two snapshots contain the same cumulative fact. Nil
// and empty per-model maps are the same set.
func (u Usage) Equal(other Usage) bool {
	if !u.Total.Equal(other.Total) || len(u.ByModel) != len(other.ByModel) {
		return false
	}
	for model, totals := range u.ByModel {
		if otherTotals, found := other.ByModel[model]; !found || !totals.Equal(otherTotals) {
			return false
		}
	}
	return true
}

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

// Snapshot is the durable usage projection for one complete execution tree.
// Models are unique and sorted by model ID so concurrent
// execution cannot make checkpoint bytes or output ordering nondeterministic.
type Snapshot struct {
	Models []ModelUsage
}

// Total returns the checked aggregate of every model in the snapshot. The
// result intentionally has an empty Model because it represents the whole
// execution subtree rather than another served model.
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
