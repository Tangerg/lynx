package agent

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// Standard-path aliases keep Agent definition and lifecycle discoverable from
// one package without copying types or hiding the advanced sub-packages. Tool,
// planning, event, persistence, and provider protocols remain at their owning
// package boundaries.
type (
	Agent          = core.Agent
	AgentConfig    = core.AgentConfig
	Action         = core.Action
	ActionConfig   = core.ActionConfig
	Goal           = core.Goal
	GoalConfig     = core.GoalConfig
	Condition      = core.Condition
	ConditionEnv   = core.ConditionEnv
	Truth          = core.Truth
	FuncCondition  = core.FuncCondition
	StuckPolicy    = core.StuckPolicy
	ProcessView    = core.ProcessView
	ProcessContext = core.ProcessContext
	ProcessOptions = core.ProcessOptions
	PromptConfig   = core.PromptConfig
	ChatCapability = core.ChatCapability
	DeploymentRef  = core.DeploymentRef
	ProcessStatus  = core.ProcessStatus
	Extension      = core.Extension
	Session        = core.Session
	RetryPolicy    = core.RetryPolicy
	RetrySafety    = core.RetrySafety

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

// Result returns the most recent T produced by process.
func Result[T any](process ProcessView) (T, bool) {
	return core.Result[T](process)
}

// NewSession returns a session initialized for a multi-turn Agent run.
func NewSession(id, userID, agentName string) Session {
	return core.NewSession(id, userID, agentName)
}

// DefaultRetryPolicy returns the safe one-attempt Action policy.
func DefaultRetryPolicy() RetryPolicy { return core.DefaultRetryPolicy() }
