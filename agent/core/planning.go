package core

import (
	"reflect"
	"time"
)

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
//
// Use [Static] to lift a constant float into a CostFunc — that single shape
// covers both static and dynamic uses, so the framework doesn't need parallel
// "static fallback" fields alongside every CostFunc field.
type CostFunc func(WorldState) float64

// Static returns a [CostFunc] that ignores the world state and always
// returns v. Use it whenever a planning cost or value is constant —
// e.g. `ActionConfig{Cost: core.Static(1.5)}`.
func Static(v float64) CostFunc {
	return func(WorldState) float64 { return v }
}

// DomainType describes a Go type that an agent treats as a planning artifact.
// It's used for documentation, schema export (e.g. JSON Schema for MCP), and
// the parent-interface walks the planner does when checking type compatibility
// between an action's outputs and a downstream action's inputs.
type DomainType struct {
	Name        string
	Description string
	ReflectType reflect.Type
	Parents     []string // Stable type names of parent interfaces (for sealed-style hierarchies).
	IsSealed    bool
}

// DomainTypeOf builds a DomainType from a generic parameter — convenient when
// declaring sealed-interface families up front so the planner has the parent
// information it needs.
func DomainTypeOf[T any](description string) DomainType {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	return DomainType{
		Name:        TypeFullName(rt),
		Description: description,
		ReflectType: rt,
	}
}
