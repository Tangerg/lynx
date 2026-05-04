// Package dsl is the user-facing fluent builder for assembling agents — the
// only entry point for defining an agent. Explicit, type-safe, no reflection
// at runtime; chosen over reflect-based registration so signature errors
// surface at compile time and IDE refactoring tools stay accurate.
//
// The split with [core.NewAgent]:
//
//   - core.NewAgent(cfg core.AgentConfig) is the bare-metal constructor —
//     one struct in, *Agent out. Use it when you already have a fully-formed
//     AgentConfig (config-driven setups, deserialised definitions).
//   - dsl.Builder is the fluent layer on top: per-field setters plus
//     slice-appending methods, ergonomic for hand-written agent definitions.
//     Build() assembles the AgentConfig and forwards to core.NewAgent.
package dsl

import (
	"github.com/Masterminds/semver/v3"

	"github.com/Tangerg/lynx/agent/core"
)

// Builder accumulates [core.AgentConfig] state across chained method calls
// and produces an immutable *core.Agent at [Builder.Build]. Partial
// builders are not safe to share across goroutines.
type Builder struct {
	cfg core.AgentConfig
}

// New starts a Builder with the given agent name. Every other field is
// optional and configurable via dedicated fluent setters
// (Description, Provider, Version, Opaque, StuckHandler) or the slice-
// appending methods (Actions, Goals, Conditions, DomainTypes,
// RequiresToolGroups). [Builder.Build] hands the assembled config to
// [core.NewAgent].
func New(name string) *Builder {
	return &Builder{cfg: core.AgentConfig{Name: name}}
}

// Description sets the agent's human-readable summary.
func (b *Builder) Description(d string) *Builder {
	b.cfg.Description = d
	return b
}

// Provider stamps the publisher / vendor metadata.
func (b *Builder) Provider(p string) *Builder {
	b.cfg.Provider = p
	return b
}

// Version sets the agent's semver tag, parsing the input string. Panics
// on a malformed literal — version strings are build-configuration, so
// a typo should fail immediately rather than at deploy time. Same model
// as [regexp.MustCompile].
func (b *Builder) Version(s string) *Builder {
	b.cfg.Version = semver.MustParse(s)
	return b
}

// Opaque flags the agent as not introspectable from the outside.
func (b *Builder) Opaque(o bool) *Builder {
	b.cfg.Opaque = o
	return b
}

// StuckHandler installs the recovery hook fired when the planner returns
// no plan. Optional — the default behaviour is to transition to
// [core.StatusStuck].
func (b *Builder) StuckHandler(h core.StuckHandler) *Builder {
	b.cfg.StuckHandler = h
	return b
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

// DomainTypes registers one or more planning-relevant types. Use when
// the agent has sealed-style interfaces and the planner needs to know
// about the parent hierarchy for type-binding lookups.
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

// Build seals the builder into an immutable *core.Agent by handing the
// assembled config to [core.NewAgent]. Callers may keep using the
// builder to construct further agents; each Build() produces a fresh
// value with its own slices.
func (b *Builder) Build() *core.Agent {
	return core.NewAgent(b.cfg)
}
