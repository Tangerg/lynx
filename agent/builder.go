package agent

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

// PlannerName selects which [planning.Planner] the runtime will use for
// this agent — must match the [core.Extension.Name] of a planner
// registered on the platform (or via process-scope extensions). An
// empty / unset value resolves to "goap". Built-in names: "goap",
// "htn", "reactive".
func (b *Builder) PlannerName(name string) *Builder {
	b.config.PlannerName = name
	return b
}

// Build seals the builder into an immutable *core.Agent. Callers may
// keep using the builder to construct further agents — Build() copies
// the accumulated config first so [core.NewAgent]'s default-filling
// (e.g. Version) doesn't leak back into the builder.
func (b *Builder) Build() *core.Agent {
	cfg := b.config
	return core.NewAgent(&cfg)
}
