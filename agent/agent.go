// Package agent is the top-level convenience surface for the Lynx agent
// framework. It re-exports every name a typical caller reaches for so
// the user-facing code can be written almost entirely against
// `agent.X` — no need to memorise which sub-package each type lives in.
//
// Sub-package map (for callers who want to pin to a single layer):
//
//	github.com/Tangerg/lynx/agent/core      — primitives (Action / Goal / Condition / Agent / Blackboard)
//	github.com/Tangerg/lynx/agent/plan      — Plan / Planner interface / PlanningSystem
//	github.com/Tangerg/lynx/agent/planner   — concrete planners (goap, …)
//	github.com/Tangerg/lynx/agent/runtime   — Platform / AgentProcess
//	github.com/Tangerg/lynx/agent/event     — event types + listener
//	github.com/Tangerg/lynx/agent/dsl       — fluent agent builder
//	github.com/Tangerg/lynx/agent/hitl      — typed Awaitable / NewConfirmation
package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/dsl"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

// ============================================================================
// Constructors / factories
// ============================================================================

// New starts a [dsl.Builder] for a fluently-defined agent.
func New(name string) *dsl.Builder { return dsl.New(name) }

// NewAction is the typed action constructor — see [core.NewAction]. Pass
// [ActionConfig]{} when defaults suffice.
func NewAction[In, Out any](name string, fn core.TypedActionFunc[In, Out], config ActionConfig) core.Action {
	return core.NewAction[In, Out](name, fn, config)
}

// NewCondition wraps a function as a [*ComputedCondition]. Returning
// the concrete pointer rather than the Condition interface follows
// Go's "accept interfaces, return structs" convention — callers can
// always assign the result to a Condition when they want the narrower
// view.
func NewCondition(name string, fn func(ctx context.Context, oc *OperationContext) Determination) *ComputedCondition {
	return core.NewCondition(name, fn)
}

// GoalProducing constructs a goal whose precondition is "an artifact of
// type T exists on the blackboard". See [core.GoalProducing].
func GoalProducing[T any](g Goal) *Goal { return core.GoalProducing[T](g) }

// NewPlatform constructs a runtime Platform from config.
func NewPlatform(config PlatformConfig) *Platform { return runtime.NewPlatform(config) }

// Static lifts a constant float into a [CostFunc] — `Cost: agent.Static(1.5)`.
func Static(v float64) CostFunc { return core.Static(v) }

// DefaultActionQoS returns the default retry/back-off policy for actions.
func DefaultActionQoS() ActionQoS { return core.DefaultActionQoS() }

// And / Or / Not compose [Condition]s with short-circuit semantics.
func And(left, right Condition) Condition { return core.And(left, right) }
func Or(left, right Condition) Condition  { return core.Or(left, right) }
func Not(inner Condition) Condition       { return core.Not(inner) }

// ============================================================================
// Top-level helpers (typed blackboard / process accessors)
// ============================================================================

// Get is a typed blackboard lookup — see [core.Get].
func Get[T any](bb BlackboardReader, name string) (T, bool) { return core.Get[T](bb, name) }

// Last returns the most-recent T on the blackboard.
func Last[T any](bb BlackboardReader) (T, bool) { return core.Last[T](bb) }

// ResultOfType pulls the most-recent T from a process's blackboard.
func ResultOfType[T any](p Process) (T, bool) { return core.ResultOfType[T](p) }

// ServiceOf is the typed service-registry lookup helper.
func ServiceOf[T any](p *ServiceProvider, key string) (T, bool) {
	return core.ServiceOf[T](p, key)
}

// ============================================================================
// Type aliases — agent.X instead of core.X / runtime.X / event.X
// ============================================================================

// --- Top-level domain objects ---
type (
	Agent             = core.Agent
	AgentConfig       = core.AgentConfig
	Goal              = core.Goal
	Action            = core.Action
	ActionMetadata    = core.ActionMetadata
	ActionConfig      = core.ActionConfig
	ActionStatus      = core.ActionStatus
	ActionQos         = core.ActionQos
	ActionQoS         = core.ActionQoS
	Condition         = core.Condition
	ComputedCondition = core.ComputedCondition
	Determination     = core.Determination
	IOBinding         = core.IOBinding
	WorldState        = core.WorldState
	CostFunc          = core.CostFunc
	EffectSpec        = core.EffectSpec
)

// --- Process surface ---
type (
	Process        = core.Process
	ProcessContext = core.ProcessContext
	ProcessOptions = core.ProcessOptions
	ProcessStatus  = core.AgentProcessStatus
	Budget         = core.Budget
	Verbosity      = core.Verbosity
	Identities     = core.Identities
	User           = core.User
	ProcessControl = core.ProcessControl

	// Hooks
	StuckHandler           = core.StuckHandler
	StuckResult            = core.StuckResult
	StuckHandlingCode      = core.StuckHandlingCode
	EarlyTerminationPolicy = core.EarlyTerminationPolicy
	MaxActionsPolicy       = core.MaxActionsPolicy
	BudgetPolicy           = core.BudgetPolicy
	CompositePolicy        = core.CompositePolicy
)

// --- Blackboard ---
type (
	Blackboard       = core.Blackboard
	BlackboardReader = core.BlackboardReader
	BlackboardWriter = core.BlackboardWriter
	OperationContext = core.OperationContext
	OutputChannel    = core.OutputChannel
	ServiceProvider  = core.ServiceProvider
)

// --- HITL ---
type (
	Awaitable      = core.Awaitable
	ResponseImpact = core.ResponseImpact
)

// --- Tools ---
type (
	AgentTool            = core.AgentTool
	ToolGroup            = core.ToolGroup
	ToolGroupResolver    = core.ToolGroupResolver
	ToolGroupRequirement = core.ToolGroupRequirement
	ToolGroupMetadata    = core.ToolGroupMetadata
	TerminationScope     = core.TerminationScope
	TerminationSignal    = core.TerminationSignal
)

// --- Replan / errors ---
type (
	ReplanRequest = core.ReplanRequest
)

// --- Runtime ---
type (
	Platform       = runtime.Platform
	PlatformConfig = runtime.PlatformConfig
	AgentProcess   = runtime.AgentProcess
	IDGenerator    = runtime.IDGenerator
	PlannerFactory = runtime.PlannerFactory
)

// --- Events ---
type (
	Event        = event.Event
	BaseEvent    = event.BaseEvent
	Listener     = event.Listener
	ListenerFunc = event.ListenerFunc
	Multicast    = event.Multicast
)

// ============================================================================
// Re-exported constants — agent.X for the most-used enum values
// ============================================================================

// Determination values (3-valued logic).
const (
	Unknown = core.Unknown
	True    = core.True
	False   = core.False
)

// ActionStatus values.
const (
	ActionSucceeded = core.ActionSucceeded
	ActionFailed    = core.ActionFailed
	ActionWaiting   = core.ActionWaiting
	ActionPaused    = core.ActionPaused
)

// AgentProcessStatus values (renamed agent.StatusX for ergonomics).
const (
	StatusNotStarted = core.StatusNotStarted
	StatusRunning    = core.StatusRunning
	StatusWaiting    = core.StatusWaiting
	StatusPaused     = core.StatusPaused
	StatusCompleted  = core.StatusCompleted
	StatusFailed     = core.StatusFailed
	StatusStuck      = core.StatusStuck
	StatusKilled     = core.StatusKilled
	StatusTerminated = core.StatusTerminated
)

// ResponseImpact values.
const (
	ResponseUnchanged = core.ResponseImpactUnchanged
	ResponseUpdated   = core.ResponseImpactUpdated
)

// StuckHandlingCode values.
const (
	StuckReplan       = core.StuckReplan
	StuckNoResolution = core.StuckNoResolution
)

// TerminationScope values.
const (
	TerminationAgent    = core.TerminationScopeAgent
	TerminationAction   = core.TerminationScopeAction
	TerminationToolCall = core.TerminationScopeToolCall
)

// PlannerType / ProcessType values.
const (
	PlannerGOAP    = core.PlannerGOAP
	PlannerUtility = core.PlannerUtility

	ProcessSequential = core.ProcessSequential
	ProcessConcurrent = core.ProcessConcurrent
)

// Binding name conventions.
const (
	DefaultBinding    = core.DefaultBindingName
	LastResultBinding = core.LastResultBindingName
)
