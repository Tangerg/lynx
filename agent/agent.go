package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
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

// NewPlatform constructs a runtime Platform from config. nil config
// yields a zero-config platform.
func NewPlatform(config *runtime.PlatformConfig) *runtime.Platform {
	return runtime.NewPlatform(config)
}
