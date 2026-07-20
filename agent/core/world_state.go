package core

import "time"

// WorldState is the planner's read-only snapshot of agent reality at one
// instant. It's defined here (rather than in plan/) so Action.Cost / Action.Value
// can take it without forcing core to import plan.
type WorldState interface {
	// Conditions returns the condition truth values. Implementations must not
	// expose mutable internal storage.
	Conditions() ConditionSet

	// Timestamp records when this snapshot was taken.
	Timestamp() time.Time

	// Key is a stable, deterministic identifier used to deduplicate visited
	// states. Two snapshots with the same condition map yield the same key.
	Key() string

	// Apply produces a new state with the supplied effects layered on top. The
	// receiver MUST NOT mutate; planners rely on snapshots being immutable.
	Apply(effects ConditionSet) WorldState
}

// ScoreFunc computes a dynamic cost or value from the current world state. The
// planner samples it during search so an action can be cheap or expensive
// depending on what's already been observed. Implementations must be
// deterministic and free of externally visible side effects because a planner
// may evaluate alternatives that are never executed. Results must be finite;
// functions used as costs must also be non-negative. A panic or invalid result
// rejects the planning pass with an attributed error.
//
// Use [FixedScore] to lift a constant float into a ScoreFunc — that single shape
// covers both static and dynamic uses, so the framework doesn't need parallel
// "static fallback" fields alongside every ScoreFunc field.
type ScoreFunc func(WorldState) float64

// FixedScore returns a [ScoreFunc] that ignores the world state and always
// returns v. Use it whenever a planning cost or value is constant —
// e.g. `ActionConfig{Cost: core.FixedScore(1.5)}`.
func FixedScore(value float64) ScoreFunc {
	return func(WorldState) float64 { return value }
}
