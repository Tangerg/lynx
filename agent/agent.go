package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning/planner/goap"
	"github.com/Tangerg/lynx/agent/planning/planner/reactive"
	"github.com/Tangerg/lynx/agent/runtime"
)

// New starts a [Builder] for a fluently-defined agent.
func New(name string) *Builder {
	return &Builder{config: core.AgentConfig{Name: name}}
}

// NewAction is the typed action constructor — see [core.NewAction]. Pass
// [core.ActionConfig]{} when defaults suffice.
func NewAction[In, Out any](name string, fn core.TypedActionFunc[In, Out], config core.ActionConfig) core.Action {
	return core.NewAction[In, Out](name, fn, config)
}

// NewCondition wraps a function as a [*core.ComputedCondition]. Returning
// the concrete pointer rather than the [core.Condition] interface follows
// Go's "accept interfaces, return structs" convention — callers can
// always assign the result to a [core.Condition] when they want the
// narrower view.
func NewCondition(name string, fn func(ctx context.Context, oc *core.ConditionEnv) core.Determination) *core.ComputedCondition {
	return core.NewCondition(name, fn)
}

// GoalProducing constructs a goal whose precondition is "an artifact of
// type T exists on the blackboard". See [core.GoalProducing].
func GoalProducing[T any](g core.Goal) *core.Goal { return core.GoalProducing[T](g) }

// ToolRolesFor builds the tool-group requirements for the given roles —
// for an [ActionConfig.ToolGroups] field. See [core.ToolRolesFor].
func ToolRolesFor(roles ...string) []ToolGroupRequirement { return core.ToolRolesFor(roles...) }

// ProcessFrom retrieves the [Process] attached to ctx (nil if absent) —
// the handle action bodies and tools read without an extra parameter.
// See [core.ProcessFrom].
func ProcessFrom(ctx context.Context) Process { return core.ProcessFrom(ctx) }

// ResultOfType pulls the most-recent T from a process's blackboard — the
// typed way to read a finished run's output. See [core.ResultOfType].
func ResultOfType[T any](p Process) (T, bool) { return core.ResultOfType[T](p) }

// Get is the typed blackboard read by (name, T). See [core.Get].
func Get[T any](bb BlackboardReader, name string) (T, bool) { return core.Get[T](bb, name) }

// Last returns the most-recent blackboard object of type T. See [core.Last].
func Last[T any](bb BlackboardReader) (T, bool) { return core.Last[T](bb) }

// NewPlatform constructs a runtime Platform from config. The zero
// value yields a default-configured platform.
//
// As the composition root, NewPlatform registers the framework's
// built-in planners (goap, reactive) as platform extensions unless the
// caller already supplied an extension of the same Name. This keeps the
// runtime package free of any concrete planner dependency — runtime
// resolves planners purely through the [planning.Planner] interface —
// while agents requesting "goap" / "reactive" (or an empty PlannerName,
// which defaults to "goap") still work out of the box. Other planners
// (htn, utility) are opt-in via config.Extensions.
func NewPlatform(config runtime.PlatformConfig) *runtime.Platform {
	config.Extensions = withDefaultPlanners(config.Extensions)
	return runtime.NewPlatform(config)
}

// withDefaultPlanners prepends the built-in goap + reactive planners to
// extensions, skipping any whose Name a caller already registered so an
// explicit override wins and the platform's duplicate-name guard never
// trips.
func withDefaultPlanners(extensions []core.Extension) []core.Extension {
	taken := make(map[string]struct{}, len(extensions))
	for _, ext := range extensions {
		if ext != nil {
			taken[ext.Name()] = struct{}{}
		}
	}
	defaults := make([]core.Extension, 0, 2)
	for _, planner := range []core.Extension{goap.NewPlanner(), reactive.NewPlanner()} {
		if _, ok := taken[planner.Name()]; !ok {
			defaults = append(defaults, planner)
		}
	}
	return append(defaults, extensions...)
}

// SnapshotProcess captures a process's full state for persistence via a
// [ProcessStore]. See [runtime.SnapshotProcess].
func SnapshotProcess(p *AgentProcess) ProcessSnapshot { return runtime.SnapshotProcess(p) }

// RestoreProcess rebuilds a process from a persisted snapshot, re-entering
// the tick loop with the same observability/session context a fresh run
// gets. See [runtime.RestoreProcess].
func RestoreProcess(platform *Platform, snap ProcessSnapshot, options ProcessOptions) (*AgentProcess, error) {
	return runtime.RestoreProcess(platform, snap, options)
}

// ChildError reports a spawned child process's terminal failure, or nil
// when it completed cleanly. See [runtime.ChildError].
func ChildError(child *AgentProcess) error { return runtime.ChildError(child) }
