package core

import "time"

// WorldState is the planner's read-only snapshot of agent reality at one
// instant. It's defined here (rather than in plan/) so Action.Cost / Action.Value
// can take it without forcing core to import plan.
type WorldState interface {
	// State exposes the condition→Determination map. Implementations return a
	// view that callers MUST NOT mutate.
	State() map[string]Determination

	// Timestamp records when this snapshot was taken.
	Timestamp() time.Time

	// HashKey is a stable, deterministic identifier used to deduplicate states
	// inside A*'s closed set. Two snapshots with the same condition map yield
	// the same key.
	HashKey() string

	// Apply produces a new state with the supplied effects layered on top. The
	// receiver MUST NOT mutate; planners rely on snapshots being immutable.
	Apply(effects EffectSpec) WorldState
}

// CostFunc computes a dynamic cost or value from the current world state. The
// planner samples it during A* search so an action can be cheap or expensive
// depending on what's already been observed.
type CostFunc func(WorldState) float64
