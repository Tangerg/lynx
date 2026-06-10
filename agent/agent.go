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
func NewCondition(name string, fn func(ctx context.Context, env *core.ConditionEnv) core.Determination) *core.ComputedCondition {
	return core.NewCondition(name, fn)
}

// GoalProducing constructs a goal whose precondition is "an artifact of
// type T exists on the blackboard". See [core.GoalProducing].
func GoalProducing[T any](g core.Goal) *core.Goal { return core.GoalProducing[T](g) }

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
