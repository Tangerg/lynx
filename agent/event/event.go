package event

import (
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// Event is the common interface — every concrete event embeds Header
// so it satisfies these methods without each type re-implementing them.
type Event interface {
	Timestamp() time.Time
	ProcessID() string
	Kind() Kind
}

// Kind identifies an event for in-memory listeners.
type Kind string

const (
	KindAgentDeployed       Kind = "agent_deployed"
	KindAgentUndeployed     Kind = "agent_undeployed"
	KindProcessCreated      Kind = "process_created"
	KindProcessCompleted    Kind = "process_completed"
	KindProcessFailed       Kind = "process_failed"
	KindProcessStuck        Kind = "process_stuck"
	KindProcessWaiting      Kind = "process_waiting"
	KindProcessKilled       Kind = "process_killed"
	KindProcessTerminated   Kind = "process_terminated"
	KindPlanningStarted     Kind = "planning_started"
	KindPlanCreated         Kind = "plan_created"
	KindReplanRequested     Kind = "replan_requested"
	KindActionStarted       Kind = "action_started"
	KindActionFinished      Kind = "action_finished"
	KindGoalAchieved        Kind = "goal_achieved"
	KindInteractionBoundary Kind = "interaction_boundary"
)

// Header is the embedded carrier shared across all concrete events.
// It's an opaque value object: built via [NewHeader] and read through
// the [Event] interface methods.
type Header struct {
	at        time.Time
	processID string
}

func (h Header) Timestamp() time.Time { return h.at }
func (h Header) ProcessID() string    { return h.processID }

// NewHeader stamps a fresh event with the current time.
func NewHeader(processID string) Header {
	return Header{at: time.Now(), processID: processID}
}

// AgentDeployed fires when an agent is registered on an Engine.
type AgentDeployed struct {
	Header
	Deployment core.DeploymentRef
}

func (AgentDeployed) Kind() Kind { return KindAgentDeployed }

// AgentUndeployed fires when an agent is removed from an Engine.
type AgentUndeployed struct {
	Header
	Deployment core.DeploymentRef
}

func (AgentUndeployed) Kind() Kind { return KindAgentUndeployed }

// ProcessCreated fires when a new Process is registered on the engine. It
// carries lifecycle identity only.
//
// It deliberately does not carry the process input. Bindings hold arbitrary
// host values, and copying the map still hands every listener the same pointers
// the first action is about to work on — mutable shared state on an observation
// channel. A host that needs the input reads it from the process it names,
// where its own concrete types are still in reach.
type ProcessCreated struct {
	Header
	// ParentID is the immediate owning process. It is empty for a root process.
	ParentID string
}

func (ProcessCreated) Kind() Kind { return KindProcessCreated }

// ProcessCompleted fires when the process reaches its goal successfully.
type ProcessCompleted struct {
	Header
	Goal core.GoalDescriptor
}

func (ProcessCompleted) Kind() Kind { return KindProcessCompleted }

// ProcessFailed fires when the process terminates with an error.
type ProcessFailed struct {
	Header
	Err error
}

func (ProcessFailed) Kind() Kind { return KindProcessFailed }

// ProcessStuck fires when the planner returns no plan and no StuckPolicy resolves it.
type ProcessStuck struct {
	Header
	State  core.WorldState
	Reason string
}

func (ProcessStuck) Kind() Kind { return KindProcessStuck }

// ProcessWaiting fires when a process parks resumable continuation state.
type ProcessWaiting struct {
	Header
	Suspension *interaction.Suspension
}

func (ProcessWaiting) Kind() Kind { return KindProcessWaiting }

func (e ProcessWaiting) cloneEvent() Event {
	if e.Suspension != nil {
		e.Suspension = e.Suspension.Clone()
	}
	return e
}

// ProcessKilled fires from Engine.Kill or when ctx is canceled mid-run.
type ProcessKilled struct {
	Header
	Reason string
}

func (ProcessKilled) Kind() Kind { return KindProcessKilled }

// ProcessTerminated fires when a StopPolicy or a queued
// [core.TerminationScopeAgent] signal stops the process.
type ProcessTerminated struct {
	Header
	Reason string
	Scope  core.TerminationScope
}

func (ProcessTerminated) Kind() Kind { return KindProcessTerminated }

// PlanningStarted reports the world state the planner is about to consume.
type PlanningStarted struct {
	Header
	State core.WorldState
}

func (PlanningStarted) Kind() Kind { return KindPlanningStarted }

// PlanCreated fires when the planner returns a non-nil plan.
type PlanCreated struct {
	Header
	Plan planning.PlanDescriptor
}

func (PlanCreated) Kind() Kind { return KindPlanCreated }

// ReplanRequested fires when an action requests another planning tick.
type ReplanRequested struct {
	Header
	ActionName string
	Reason     string
}

func (ReplanRequested) Kind() Kind { return KindReplanRequested }

// ActionStarted fires before an action is invoked. Listeners receive an inert
// descriptor: an observation channel has no business handing out executable
// actions or planner score functions.
type ActionStarted struct {
	Header
	Action    core.ActionDescriptor
	StartedAt time.Time
}

func (ActionStarted) Kind() Kind { return KindActionStarted }

// ActionFinished fires after an action invocation terminates. Action carries the
// description for the same reason as [ActionStarted].
type ActionFinished struct {
	Header
	Action   core.ActionDescriptor
	Status   core.ActionStatus
	Duration time.Duration
	Err      error
}

func (ActionFinished) Kind() Kind { return KindActionFinished }

// GoalAchieved fires when the planner returns an empty plan for a non-nil goal.
type GoalAchieved struct {
	Header
	Goal core.GoalDescriptor
}

func (GoalAchieved) Kind() Kind { return KindGoalAchieved }

// InteractionBoundary binds one model/tool protocol event to the exact process
// deployment and logical interaction that produced it.
type InteractionBoundary struct {
	Header
	Deployment    core.DeploymentRef
	InteractionID string
	Boundary      interaction.Event
}

func (InteractionBoundary) Kind() Kind { return KindInteractionBoundary }

func (e InteractionBoundary) cloneEvent() Event {
	e.Boundary = e.Boundary.Clone()
	return e
}
