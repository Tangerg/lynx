package core

import (
	"context"
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
	Preconditions Effects
	Effects       Effects
	CanRerun      bool
	QoS           ActionQoS
	ToolGroups    []ToolGroupRequirement

	// Cost defaults to [Static](1.0) so the planner doesn't pick
	// "free" actions over ones with real work.
	Cost CostFunc

	// Value defaults to [Static](0).
	Value CostFunc

	ClearBlackboard bool // On success, clear blackboard before binding output.
}

// HasRunPrefix prefixes the conventional "this action has run" condition
// keys ([ActionMetadata.EffectiveRunKey] mints them; the runtime's world-
// state determiner routes any key with this prefix to the blackboard's
// condition map). One constant on the protocol type so the producer and
// the classifier can't drift.
const HasRunPrefix = "hasRun_"

// EffectiveRunKey is the conventional condition key recording that this
// action has executed at least once. The runtime sets it after each
// successful run; the planner consumes it as a precondition guard for
// non-rerunnable actions.
func (m ActionMetadata) EffectiveRunKey() string {
	return HasRunPrefix + m.Name
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

// ActionQoS governs retry behavior for a single action. Retry math
// (exponential backoff, jitter) is delegated to
// [github.com/Tangerg/lynx/pkg/retry].
type ActionQoS struct {
	// MaxAttempts caps total tries (initial + retries). 0 → default.
	MaxAttempts int
	// BaseDelay is the initial wait; successive attempts grow ×2 up
	// to MaxDelay with jitter.
	BaseDelay time.Duration
	// MaxDelay caps the per-attempt wait. 0 means uncapped.
	MaxDelay time.Duration
}

// DefaultActionQoS returns sensible production defaults: 5 attempts,
// 10s initial backoff, 60s cap — aggressive because LLM calls fail
// transiently more often than typical RPC.
func DefaultActionQoS() ActionQoS {
	return ActionQoS{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Second,
		MaxDelay:    60 * time.Second,
	}
}

// Effects maps condition keys to required (or produced)
// Determinations. Used for both [ActionMetadata.Preconditions] /
// [ActionMetadata.Effects] and [Goal] preconditions. A nil Effects
// is a valid empty value.
type Effects map[string]Determination
