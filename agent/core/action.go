package core

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"time"
)

// Action is the agent's smallest planning unit. Implementations are
// typically produced via [NewAction] so the framework keeps type
// information end-to-end; the interface form is here for advanced users
// who want hand-rolled control over Execute (e.g. plugging into
// non-typed integrations).
type Action interface {
	Metadata() ActionMetadata
	// Execute runs the action body. It returns ActionStatus instead of an
	// error directly because some non-success outcomes (waiting, paused) are
	// not failures and the runtime needs to distinguish them.
	Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}

// ActionMetadata is everything the planner needs to reason about an
// action without invoking it. Immutable after construction.
//
// Cost and Value are [CostFunc]s rather than (static, fn) pairs so the
// planner has one uniform invocation point. Use [Static] to lift a
// constant — e.g. `Cost: core.Static(1.0)` — when no state-dependent
// math is needed.
type ActionMetadata struct {
	Name          string
	Description   string
	Inputs        []IOBinding
	Outputs       []IOBinding
	Preconditions EffectSpec
	Effects       EffectSpec
	CanRerun      bool
	ReadOnly      bool
	QoS           ActionQoS
	ToolGroups    []ToolGroupRequirement

	// Cost is the planner's per-tick cost probe; defaults to
	// [Static](1.0) so the planner doesn't accidentally pick "free"
	// actions in preference to ones with real work to do.
	Cost CostFunc

	// Value is the planner's per-tick value probe; defaults to
	// [Static](0).
	Value CostFunc

	Trigger         reflect.Type // Optional — autostart this action when the trigger type appears.
	OutputBinding   string       // Override the variable name written to the blackboard.
	ClearBlackboard bool         // On success, clear blackboard before binding output.
}

// HasRunKey is the conventional condition key recording that this
// action has executed at least once. The runtime sets it after each
// successful run; the planner consumes it as a precondition guard for
// non-rerunnable actions.
func (m ActionMetadata) HasRunKey() string {
	return "hasRun_" + m.Name
}

// IsApplicableIn reports whether every precondition holds in state.
// Used by the concurrent runner to filter the plan's actions to those
// currently runnable on this tick.
func (m ActionMetadata) IsApplicableIn(state map[string]Determination) bool {
	for key, required := range m.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}

// ActionQoS governs retry behavior for a single action. Retry math itself
// (exponential backoff, jitter, overflow protection) is delegated to
// [github.com/Tangerg/lynx/pkg/retry]; this struct is just the policy
// surface the runtime translates into [retry.Option] values.
//
// Defaults are taken from embabel — aggressive (5 attempts) because LLM
// calls fail transiently more often than typical RPC.
type ActionQoS struct {
	// MaxAttempts caps total tries (initial + retries). 0 falls back to
	// the package default; the runtime treats anything < 1 as 1.
	MaxAttempts int

	// BaseDelay is the initial wait between attempts. Successive
	// attempts grow this exponentially (×2 per step) up to MaxDelay,
	// with random jitter added on each attempt.
	BaseDelay time.Duration

	// MaxDelay caps the per-attempt wait. 0 means uncapped.
	MaxDelay time.Duration
}

// DefaultActionQoS returns sensible production defaults: 5 attempts, 10s
// initial backoff, 60s cap.
func DefaultActionQoS() ActionQoS {
	return ActionQoS{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Second,
		MaxDelay:    60 * time.Second,
	}
}

// EffectSpec maps condition keys to required (or produced) Determinations.
// It represents both Action.Preconditions ("what must hold before I can run")
// and Action.Effects ("what becomes true after I run"). The same shape is
// reused for Goal.Preconditions.
//
// A nil EffectSpec is a valid, read-only empty value; all helpers accept it
// without panicking.
type EffectSpec map[string]Determination

// Clone returns a deep copy. A nil receiver yields nil so callers can chain
// "spec.Clone()" without a guard.
func (s EffectSpec) Clone() EffectSpec {
	if s == nil {
		return nil
	}
	return maps.Clone(s)
}

// Merge layers other on top of s — keys in other win. Returns a new map;
// neither input is modified.
func (s EffectSpec) Merge(other EffectSpec) EffectSpec {
	out := make(EffectSpec, len(s)+len(other))
	maps.Copy(out, s)
	maps.Copy(out, other)
	return out
}

// Keys returns sorted condition keys for stable iteration (used by HashKey
// computations and debug printing).
func (s EffectSpec) Keys() []string {
	out := slices.Collect(maps.Keys(s))
	slices.Sort(out)
	return out
}

// Set returns a copy with key=value applied — intended for fluent
// construction, not for mutating an existing spec in place.
func (s EffectSpec) Set(key string, value Determination) EffectSpec {
	out := s.Clone()
	if out == nil {
		out = EffectSpec{}
	}
	out[key] = value
	return out
}
