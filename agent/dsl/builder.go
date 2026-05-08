// Package dsl is the user-facing fluent builder for assembling agents.
// Explicit, type-safe, no reflection — chosen over reflect-based
// registration so signature errors surface at compile time and IDE
// refactoring tools stay accurate.
//
// The split with [core.NewAgent]: the bare-metal constructor takes a
// [core.AgentConfig] directly; [Builder] is a fluent layer that
// accumulates the same fields and calls [core.NewAgent] on
// [Builder.Build]. Use the bare constructor for config-driven /
// deserialised definitions; the Builder for hand-written code.
package dsl

import (
	"github.com/Masterminds/semver/v3"

	"github.com/Tangerg/lynx/agent/core"
)

// Builder accumulates [core.AgentConfig] state across chained method
// calls and produces an immutable *core.Agent at [Builder.Build].
// Partial builders are not safe to share across goroutines.
type Builder struct {
	config core.AgentConfig
}

// New starts a Builder with the given agent name.
func New(name string) *Builder {
	return &Builder{config: core.AgentConfig{Name: name}}
}

func (b *Builder) Description(description string) *Builder {
	b.config.Description = description
	return b
}

// Version parses version as semver and sets it on the agent. Panics
// on a malformed literal — same model as [regexp.MustCompile].
func (b *Builder) Version(version string) *Builder {
	b.config.Version = semver.MustParse(version)
	return b
}

// StuckHandler installs the recovery hook fired when the planner
// returns no plan; nil leaves the default ("transition to
// [core.StatusStuck]").
func (b *Builder) StuckHandler(handler core.StuckHandler) *Builder {
	b.config.StuckHandler = handler
	return b
}

func (b *Builder) Actions(actions ...core.Action) *Builder {
	b.config.Actions = append(b.config.Actions, actions...)
	return b
}

func (b *Builder) Goals(goals ...*core.Goal) *Builder {
	b.config.Goals = append(b.config.Goals, goals...)
	return b
}

func (b *Builder) Conditions(conditions ...core.Condition) *Builder {
	b.config.Conditions = append(b.config.Conditions, conditions...)
	return b
}

// Build seals the builder into an immutable *core.Agent. Callers may
// keep using the builder to construct further agents; each Build()
// produces a fresh value with its own slices.
func (b *Builder) Build() *core.Agent {
	return core.NewAgent(b.config)
}
