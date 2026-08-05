package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/chat"
)

// Standard-path aliases keep Agent definition and lifecycle discoverable from
// one package without copying types or hiding the advanced sub-packages. Tool,
// planning, event, interaction, and provider protocols remain at their owning
// package boundaries.
type (
	Agent              = core.Agent
	Config             = core.AgentConfig
	Descriptor         = core.AgentDescriptor
	Action             = core.Action
	ActionConfig       = core.ActionConfig
	Goal               = core.Goal
	GoalConfig         = core.GoalConfig
	Condition          = core.Condition
	ConditionConfig    = core.ConditionConfig
	ConditionEnv       = core.ConditionEnv
	Truth              = core.Truth
	FuncCondition      = core.FuncCondition
	StuckPolicy        = core.StuckPolicy
	ProcessView        = core.ProcessView
	ProcessContext     = core.ProcessContext
	ProcessOptions     = core.ProcessOptions
	ConfigureChildFunc = core.ConfigureChildFunc
	PromptConfig       = core.PromptConfig
	ChatCapability     = core.ChatCapability
	DeploymentRef      = core.DeploymentRef
	ProcessStatus      = core.ProcessStatus
	Extension          = core.Extension
	Bindings           = core.Bindings

	ToolGroup         = core.ToolGroup
	ToolGroupResolver = core.ToolGroupResolver

	Suspension            = interaction.Suspension
	InteractionEvent      = interaction.Event
	InteractionEventKind  = interaction.EventKind
	InteractionLimits     = interaction.Limits
	InteractionStopReason = interaction.StopReason

	Engine        = runtime.Engine
	EngineConfig  = runtime.Config
	Deployment    = runtime.Deployment
	Process       = runtime.Process
	RunHandle     = runtime.RunHandle
	RunCompletion = runtime.RunCompletion
)

// FuncAction is [core.FuncAction]. It stands apart from the aliases above only
// because it is generic.
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
)

const (
	SuspensionSchemaVersion = interaction.SuspensionSchemaVersion

	InteractionEventModelRequest  = interaction.EventModelRequest
	InteractionEventModelResponse = interaction.EventModelResponse
	InteractionEventToolCall      = interaction.EventToolCall
	InteractionEventToolResult    = interaction.EventToolResult
	InteractionEventPause         = interaction.EventPause
	InteractionEventResume        = interaction.EventResume

	InteractionStopNone       = interaction.StopNone
	InteractionStopBudget     = interaction.StopBudget
	InteractionStopModelCalls = interaction.StopModelCalls
)

// New constructs a read-only Agent definition from ordinary Go config.
func New(config Config) *Agent { return core.NewAgent(config) }

// Input returns initial blackboard bindings for one conventional input value.
func Input(value any) Bindings { return core.Input(value) }

// NewGoal constructs an immutable Goal from ordinary Go config.
func NewGoal(config GoalConfig) *Goal { return core.NewGoal(config) }

// NewAction constructs a typed function-backed action. Pass [ActionConfig]{}
// when defaults suffice.
func NewAction[In, Out any](name string, fn func(context.Context, *ProcessContext, In) (Out, error), config ActionConfig) *FuncAction[In, Out] {
	return core.NewAction[In, Out](name, core.ActionFunc[In, Out](fn), config)
}

// NewCondition constructs a function-backed condition.
func NewCondition(config ConditionConfig) *FuncCondition {
	return core.NewCondition(config)
}

// NewOutputGoal constructs a goal whose precondition is an artifact of type T
// on the blackboard.
func NewOutputGoal[T any](config GoalConfig) *Goal { return core.NewOutputGoal[T](config) }

// Result returns the most recent T produced by process.
func Result[T any](process ProcessView) (T, bool) {
	return core.Result[T](process)
}

// CompletionResult returns the most recent T captured by completion.
func CompletionResult[T any](completion RunCompletion) (T, bool) {
	return runtime.CompletionResult[T](completion)
}

// Chat wires one model into a [ChatCapability], enabling streaming when the
// model also implements [chat.Streamer] — the usual case for a single client.
// Build the struct directly only to pair a distinct Model and Streamer.
func Chat(model chat.Model) ChatCapability {
	capability := ChatCapability{Model: model}
	if streamer, ok := model.(chat.Streamer); ok {
		capability.Streamer = streamer
	}
	return capability
}

// RequireType returns the precondition key for "a value of type T is present on
// the default binding", for use in [ActionConfig].Preconditions or
// [GoalConfig].RequiredConditions. It replaces hand-built binding-key strings.
func RequireType[T any]() string {
	return core.NewBinding[T]("").String()
}

// Get returns the most recent value of type T stored under name on the process
// blackboard. Pass [DefaultBindingName] for the conventional input slot.
func Get[T any](blackboard core.BlackboardReader, name string) (T, bool) {
	return core.Get[T](blackboard, name)
}
