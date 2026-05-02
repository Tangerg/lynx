// Package dsl is the user-facing fluent builder for assembling agents — the
// only entry point for defining an agent. Explicit, type-safe, no reflection
// at runtime; chosen over reflect-based registration so signature errors
// surface at compile time and IDE refactoring tools stay accurate.
package dsl

import (
	"github.com/Masterminds/semver/v3"

	"github.com/Tangerg/lynx/agent/core"
)

// Builder assembles a *core.Agent through chained method calls. Build()
// produces the immutable result; partial Builders are not safe to share
// across goroutines.
type Builder struct {
	meta       core.AgentMeta
	actions    []core.Action
	goals      []*core.Goal
	conditions []core.Condition

	domainTypes           []core.DomainType
	toolGroupRequirements []core.ToolGroupRequirement
}

// defaultVersion is the implicit Agent.Version when the caller never calls
// [Builder.Version]. Parsed once at package init via [semver.MustParse].
var defaultVersion = semver.MustParse("1.0.0")

// New starts a Builder with the given agent name. Default version is 1.0.0.
func New(name string) *Builder {
	return &Builder{
		meta: core.AgentMeta{
			Name:    name,
			Version: defaultVersion,
		},
	}
}

// Description sets the human-facing description.
func (b *Builder) Description(d string) *Builder {
	b.meta.Description = d
	return b
}

// Provider stamps the publisher / vendor.
func (b *Builder) Provider(p string) *Builder {
	b.meta.Provider = p
	return b
}

// Version sets the agent's semver. Panics on a malformed literal — version
// strings come from build configuration, so a typo is a programmer error
// that should surface immediately rather than at runtime.
func (b *Builder) Version(s string) *Builder {
	b.meta.Version = semver.MustParse(s)
	return b
}

// Opaque flags the agent as not-introspectable from the outside.
func (b *Builder) Opaque(opaque bool) *Builder {
	b.meta.Opaque = opaque
	return b
}

// StuckHandler sets the recovery hook fired when the planner returns no
// plan. Optional — the default is "transition to StatusStuck".
func (b *Builder) StuckHandler(h core.StuckHandler) *Builder {
	b.meta.StuckHandler = h
	return b
}

// Actions appends one or more actions.
func (b *Builder) Actions(actions ...core.Action) *Builder {
	b.actions = append(b.actions, actions...)
	return b
}

// Goals appends one or more goals.
func (b *Builder) Goals(goals ...*core.Goal) *Builder {
	b.goals = append(b.goals, goals...)
	return b
}

// Conditions appends one or more conditions.
func (b *Builder) Conditions(conditions ...core.Condition) *Builder {
	b.conditions = append(b.conditions, conditions...)
	return b
}

// DomainTypes registers one or more planning-relevant types. Use when the
// agent has sealed-style interfaces and the planner needs to know about the
// parent hierarchy for type-binding lookups.
func (b *Builder) DomainTypes(types ...core.DomainType) *Builder {
	b.domainTypes = append(b.domainTypes, types...)
	return b
}

// RequiresToolGroups declares one or more agent-scoped tool group
// requirements. Per-action requirements live on the Action itself.
func (b *Builder) RequiresToolGroups(reqs ...core.ToolGroupRequirement) *Builder {
	b.toolGroupRequirements = append(b.toolGroupRequirements, reqs...)
	return b
}

// Build seals the builder into an immutable *core.Agent. Callers may keep
// using the builder to construct further agents; each Build() produces a
// fresh value with its own slices.
func (b *Builder) Build() *core.Agent {
	agent := core.NewAgent(b.meta, b.actions, b.goals, b.conditions)
	agent.DomainTypes = b.domainTypes
	agent.ToolGroupRequirements = b.toolGroupRequirements
	return agent
}
