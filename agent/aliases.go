package agent

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// This file promotes the framework's user-facing types and constants from
// the internal layering — core (primitives) and runtime (engine) — into
// this single package, so a typical caller imports only `agent` to define,
// run, extend, and persist an agent. The aliases are zero-cost (identical
// types); core / runtime stay importable directly for advanced or
// extension use, and consumers remain free to depend on their own narrow
// interfaces over these types (the framework only promotes them for
// discoverability — see doc.go).

// --- Agent definition ---
type (
	Agent         = core.Agent
	AgentConfig   = core.AgentConfig
	Action        = core.Action
	ActionConfig  = core.ActionConfig
	ActionQoS     = core.ActionQoS
	Goal          = core.Goal
	Condition     = core.Condition
	ConditionEnv  = core.ConditionEnv
	Determination = core.Determination
	WorldState    = core.WorldState
)

// --- The surface an Action body sees ---
type (
	ProcessContext   = core.ProcessContext
	Process          = core.Process
	Blackboard       = core.Blackboard
	BlackboardReader = core.BlackboardReader
	BlackboardWriter = core.BlackboardWriter
)

// --- Extension points: implement one and register it as an Extension ---
type (
	Extension          = core.Extension
	ActionMiddleware   = core.ActionMiddleware
	ToolDecorator      = core.ToolDecorator
	AgentValidator     = core.AgentValidator
	GoalApprover       = core.GoalApprover
	ChatClientProvider = core.ChatClientProvider
	StuckPolicy        = core.StuckPolicy
	IDGenerator        = core.IDGenerator
	EventListener      = runtime.EventListener
)

// --- Tool grouping ---
type (
	AgentTool               = core.AgentTool
	ToolGroup               = core.ToolGroup
	ToolGroupResolver       = core.ToolGroupResolver
	ToolGroupMetadata       = core.ToolGroupMetadata
	ToolGroupRequirement    = core.ToolGroupRequirement
	SimpleToolGroupMetadata = core.SimpleToolGroupMetadata
)

// --- Human-in-the-loop ---
type (
	Awaitable      = core.Awaitable
	ResponseImpact = core.ResponseImpact
)

// --- Run + persistence ---
type (
	Platform           = runtime.Platform
	PlatformConfig     = runtime.PlatformConfig
	AgentProcess       = runtime.AgentProcess
	ProcessOptions     = core.ProcessOptions
	ProcessSnapshot    = core.ProcessSnapshot
	ProcessStore       = core.ProcessStore
	AgentProcessStatus = core.AgentProcessStatus
	LLMInvocation      = core.LLMInvocation
	Guardrails         = core.Guardrails
	Session            = core.Session
	SessionStore       = core.SessionStore
)

// --- AgentProcessStatus values ---
const (
	StatusNotStarted = core.StatusNotStarted
	StatusRunning    = core.StatusRunning
	StatusCompleted  = core.StatusCompleted
	StatusFailed     = core.StatusFailed
	StatusWaiting    = core.StatusWaiting
	StatusPaused     = core.StatusPaused
	StatusTerminated = core.StatusTerminated
	StatusKilled     = core.StatusKilled
)

// --- Determination values (three-valued logic; zero value is Unknown) ---
const (
	Unknown = core.Unknown
	True    = core.True
	False   = core.False
)

// --- ResponseImpact values ---
const (
	ImpactUnchanged = core.ImpactUnchanged
	ImpactUpdated   = core.ImpactUpdated
)

// --- Persistence sentinels (errors.Is-comparable) ---
var (
	ErrSessionNotFound  = core.ErrSessionNotFound
	ErrSnapshotNotFound = core.ErrSnapshotNotFound
)
