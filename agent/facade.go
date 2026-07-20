package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
)

// Standard-path aliases keep Agent definition and lifecycle discoverable from
// one package without copying types or hiding the advanced sub-packages. Tool,
// planning, event, persistence, and provider protocols remain at their owning
// package boundaries.
type (
	Agent            = core.Agent
	AgentConfig      = core.AgentConfig
	Action           = core.Action
	ActionConfig     = core.ActionConfig
	Goal             = core.Goal
	GoalConfig       = core.GoalConfig
	Condition        = core.Condition
	ConditionEnv     = core.ConditionEnv
	Truth            = core.Truth
	FuncCondition    = core.FuncCondition
	StuckPolicy      = core.StuckPolicy
	ProcessView      = core.ProcessView
	ProcessContext   = core.ProcessContext
	ProcessOptions   = core.ProcessOptions
	ChildOptionsFunc = core.ChildOptionsFunc
	PromptConfig     = core.PromptConfig
	ChatCapability   = core.ChatCapability
	DeploymentRef    = core.DeploymentRef
	ProcessStatus    = core.ProcessStatus
	Extension        = core.Extension
	Session          = core.Session
	RetryPolicy      = core.RetryPolicy
	RetrySafety      = core.RetrySafety

	Suspension            = interaction.Suspension
	SuspensionKind        = interaction.SuspensionKind
	InteractionEvent      = interaction.Event
	InteractionEventKind  = interaction.EventKind
	InteractionLimits     = interaction.Limits
	InteractionStopReason = interaction.StopReason

	Engine       = runtime.Engine
	EngineConfig = runtime.Config
	Deployment   = runtime.Deployment
	Process      = runtime.Process
)

type FuncAction[In, Out any] = core.FuncAction[In, Out]

const (
	DefaultBindingName = core.DefaultBindingName

	Unknown = core.Unknown
	True    = core.True
	False   = core.False

	StatusNotStarted = core.StatusNotStarted
	StatusRunning    = core.StatusRunning
	StatusCompleted  = core.StatusCompleted
	StatusFailed     = core.StatusFailed
	StatusStuck      = core.StatusStuck
	StatusWaiting    = core.StatusWaiting
	StatusPaused     = core.StatusPaused
	StatusTerminated = core.StatusTerminated
	StatusKilled     = core.StatusKilled

	RetrySafetyUnspecified = core.RetrySafetyUnspecified
	RetrySafetyIdempotent  = core.RetrySafetyIdempotent
	RetrySafetyCompensated = core.RetrySafetyCompensated
)

const (
	SuspensionSchemaVersion = interaction.SuspensionSchemaVersion
	SuspensionHuman         = interaction.SuspensionHuman
	SuspensionTool          = interaction.SuspensionTool

	InteractionEventModelRequest  = interaction.EventModelRequest
	InteractionEventModelResponse = interaction.EventModelResponse
	InteractionEventToolCall      = interaction.EventToolCall
	InteractionEventToolResult    = interaction.EventToolResult
	InteractionEventPause         = interaction.EventPause
	InteractionEventResume        = interaction.EventResume

	InteractionStopNone   = interaction.StopNone
	InteractionStopBudget = interaction.StopBudget
	InteractionStopSteps  = interaction.StopSteps
)

// New constructs a read-only Agent definition from ordinary Go config.
func New(config AgentConfig) *Agent { return core.NewAgent(config) }

// NewGoal constructs an immutable Goal from ordinary Go config.
func NewGoal(config GoalConfig) *Goal { return core.NewGoal(config) }

// NewAction constructs a typed function-backed action. Pass [ActionConfig]{}
// when defaults suffice.
func NewAction[In, Out any](name string, fn func(context.Context, *ProcessContext, In) (Out, error), config ActionConfig) *FuncAction[In, Out] {
	return core.NewAction[In, Out](name, core.ActionFunc[In, Out](fn), config)
}

// NewCondition constructs a function-backed condition.
func NewCondition(name string, fn func(context.Context, *ConditionEnv) Truth) *FuncCondition {
	return core.NewCondition(name, fn)
}

// NewOutputGoal constructs a goal whose precondition is an artifact of type T
// on the blackboard.
func NewOutputGoal[T any](config GoalConfig) *Goal { return core.NewOutputGoal[T](config) }

// Result returns the most recent T produced by process.
func Result[T any](process ProcessView) (T, bool) {
	return core.Result[T](process)
}

// PromptJSON requests JSON matching T through the process model and tool loop.
func PromptJSON[T any](ctx context.Context, process *ProcessContext, text string, config PromptConfig) (T, error) {
	return core.PromptJSON[T](ctx, process, text, config)
}

// NewSession returns a session initialized for a multi-turn Agent run.
func NewSession(id, userID, agentName string) Session {
	return core.NewSession(id, userID, agentName)
}

// DefaultRetryPolicy returns the safe one-attempt Action policy.
func DefaultRetryPolicy() RetryPolicy { return core.DefaultRetryPolicy() }
