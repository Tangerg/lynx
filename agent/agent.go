// Package agent is the top-level convenience surface for the Lynx agent
// framework. It re-exports the most-used names from core / dsl / runtime so
// callers can write `agent.New(...)` and `agent.NewPlatform(...)` without
// importing five subpackages.
//
// The full surface area lives in:
//
//	github.com/Tangerg/lynx/agent/core      — primitives, Action/Goal/Condition/Agent
//	github.com/Tangerg/lynx/agent/plan      — WorldState, Plan, Planner interface
//	github.com/Tangerg/lynx/agent/planner   — concrete planners (goap, ...)
//	github.com/Tangerg/lynx/agent/runtime   — Platform, AgentProcess
//	github.com/Tangerg/lynx/agent/event     — event types and listener
//	github.com/Tangerg/lynx/agent/dsl       — fluent agent builder
//	github.com/Tangerg/lynx/agent/hitl      — typed Awaitable / Confirmation / Form
package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/dsl"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

// --- DSL ------------------------------------------------------------------

// New starts a Builder for a fluently-defined agent.
func New(name string) *dsl.Builder { return dsl.New(name) }

// --- Action / Goal / Condition shortcuts ---------------------------------

// NewAction is the typed action constructor (see core.NewAction). Pass
// [core.ActionConfig]{} when defaults suffice.
func NewAction[In, Out any](name string, fn core.TypedActionFunc[In, Out], cfg core.ActionConfig) core.Action {
	return core.NewAction[In, Out](name, fn, cfg)
}

// NewCondition wraps a function as a Condition.
func NewCondition(name string, fn func(ctx context.Context, oc *core.OperationContext) core.Determination) *core.ComputedCondition {
	return core.NewCondition(name, fn)
}

// GoalProducing constructs a goal whose precondition is "an artifact of
// type T exists on the blackboard".
func GoalProducing[T any](description string) *core.Goal {
	return core.GoalProducing[T](description)
}

// --- Platform ------------------------------------------------------------

// NewPlatform constructs a runtime Platform with the supplied options.
func NewPlatform(opts ...runtime.PlatformOption) *runtime.Platform {
	return runtime.NewPlatform(opts...)
}

// WithChatClient attaches an LLM client to the platform.
func WithChatClient(c core.ChatClient) runtime.PlatformOption {
	return runtime.WithChatClient(c)
}

// WithListener pre-attaches an event listener at platform construction.
func WithListener(l event.Listener) runtime.PlatformOption {
	return runtime.WithListener(l)
}

// --- Re-exports useful for callers ---------------------------------------

// Type aliases keep call-site code short.
type (
	Agent          = core.Agent
	Goal           = core.Goal
	Action         = core.Action
	ActionMetadata = core.ActionMetadata
	Condition      = core.Condition
	Blackboard     = core.Blackboard
	WorldState     = core.WorldState
	Process        = core.Process
	ProcessContext = core.ProcessContext
	Determination  = core.Determination

	Platform     = runtime.Platform
	AgentProcess = runtime.AgentProcess
)
