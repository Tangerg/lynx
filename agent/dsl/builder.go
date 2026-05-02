// Package dsl is the user-facing fluent builder for assembling agents — the
// only entry point for defining an agent. Explicit, type-safe, no reflection
// at runtime; chosen over reflect-based registration so signature errors
// surface at compile time and IDE refactoring tools stay accurate.
//
// The builder is a thin façade over [core.NewAgent] — it lets callers spread
// the config across multiple statements (useful when actions/goals are
// produced conditionally). Callers who already have a fully-formed
// [core.AgentConfig] in hand can skip the builder and call [core.NewAgent]
// directly.
package dsl

import (
	"github.com/Tangerg/lynx/agent/core"
)

// Builder accumulates [core.AgentConfig] state across chained method calls
// and produces an immutable *core.Agent at [Builder.Build]. Partial
// builders are not safe to share across goroutines.
type Builder struct {
	cfg core.AgentConfig
}

// New starts a Builder seeded with cfg. Subsequent slice-appending methods
// (Actions / Goals / Conditions / DomainTypes / RequiresToolGroups) extend
// whatever cfg already contains, so callers can mix literal and chained
// styles freely. Empty cfg.Version falls back to 1.0.0 in
// [core.NewAgent].
func New(cfg core.AgentConfig) *Builder {
	return &Builder{cfg: cfg}
}

// Actions appends one or more actions.
func (b *Builder) Actions(actions ...core.Action) *Builder {
	b.cfg.Actions = append(b.cfg.Actions, actions...)
	return b
}

// Goals appends one or more goals.
func (b *Builder) Goals(goals ...*core.Goal) *Builder {
	b.cfg.Goals = append(b.cfg.Goals, goals...)
	return b
}

// Conditions appends one or more conditions.
func (b *Builder) Conditions(conditions ...core.Condition) *Builder {
	b.cfg.Conditions = append(b.cfg.Conditions, conditions...)
	return b
}

// DomainTypes registers one or more planning-relevant types. Use when the
// agent has sealed-style interfaces and the planner needs to know about
// the parent hierarchy for type-binding lookups.
func (b *Builder) DomainTypes(types ...core.DomainType) *Builder {
	b.cfg.DomainTypes = append(b.cfg.DomainTypes, types...)
	return b
}

// RequiresToolGroups declares one or more agent-scoped tool group
// requirements. Per-action requirements live on the Action itself.
func (b *Builder) RequiresToolGroups(reqs ...core.ToolGroupRequirement) *Builder {
	b.cfg.ToolGroupRequirements = append(b.cfg.ToolGroupRequirements, reqs...)
	return b
}

// Build seals the builder into an immutable *core.Agent. Callers may keep
// using the builder to construct further agents; each Build() produces a
// fresh value with its own slices.
func (b *Builder) Build() *core.Agent {
	return core.NewAgent(b.cfg)
}
