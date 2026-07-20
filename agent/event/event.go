package event

import (
	"encoding/json"
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
	Kind() string
}

// Header is the embedded carrier shared across all concrete events.
// It's an opaque value object: built via [NewHeader] and read through
// the [Event] interface methods. The timestamp / process id reach the
// wire via [emit]'s envelope (which reads them through Timestamp() /
// ProcessID()), so the fields carry no JSON tags of their own.
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
	Deployment core.DeploymentRef `json:"deployment"`
}

func (AgentDeployed) Kind() string { return "agent_deployed" }

// AgentUndeployed fires when an agent is removed from an Engine.
type AgentUndeployed struct {
	Header
	Deployment core.DeploymentRef `json:"deployment"`
}

func (AgentUndeployed) Kind() string { return "agent_undeployed" }

// ProcessCreated fires when a new Process is registered on the engine.
type ProcessCreated struct {
	Header
	Bindings core.Bindings `json:"bindings,omitzero"`
}

func (ProcessCreated) Kind() string { return "process_created" }

// ProcessCompleted fires when the process reaches its goal successfully.
type ProcessCompleted struct {
	Header
	Goal   *core.Goal `json:"-"`
	Result any        `json:"-"`
}

func (ProcessCompleted) Kind() string { return "process_completed" }

// ProcessFailed fires when the process terminates with an error.
type ProcessFailed struct {
	Header
	Err error `json:"-"`
}

func (ProcessFailed) Kind() string { return "process_failed" }

// ProcessStuck fires when the planner returns no plan and no StuckPolicy resolves it.
type ProcessStuck struct {
	Header
	State core.WorldState `json:"-"`
}

func (ProcessStuck) Kind() string { return "process_stuck" }

// ProcessWaiting fires when a process parks durable continuation state.
type ProcessWaiting struct {
	Header
	Suspension *interaction.Suspension `json:"suspension"`
}

func (ProcessWaiting) Kind() string { return "process_waiting" }

// ProcessSnapshotFailed reports that automatic persistence did not commit.
// Report-only policy means the process may continue but is explicitly
// degraded rather than being represented as durable.
type ProcessSnapshotFailed struct {
	Header
	Policy string `json:"policy"`
	Err    error  `json:"-"`
}

func (ProcessSnapshotFailed) Kind() string { return "process_snapshot_failed" }

// ProcessKilled fires from Engine.Kill or when ctx is canceled mid-run.
type ProcessKilled struct {
	Header
	Reason string `json:"reason,omitempty"`
}

func (ProcessKilled) Kind() string { return "process_killed" }

// ProcessTerminated fires when a StopPolicy or a queued
// [core.TerminationScopeAgent] signal stops the process.
type ProcessTerminated struct {
	Header
	Reason string                `json:"reason,omitempty"`
	Scope  core.TerminationScope `json:"-"`
}

func (ProcessTerminated) Kind() string { return "process_terminated" }

// PlanningStarted reports the world state the planner is about to consume.
type PlanningStarted struct {
	Header
	State core.WorldState `json:"-"`
}

func (PlanningStarted) Kind() string { return "planning_started" }

// PlanCreated fires when the planner returns a non-nil plan.
type PlanCreated struct {
	Header
	Plan *planning.Plan `json:"-"`
}

func (PlanCreated) Kind() string { return "plan_created" }

// ReplanRequested fires when an action requests another planning tick.
type ReplanRequested struct {
	Header
	ActionName string `json:"action,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

func (ReplanRequested) Kind() string { return "replan_requested" }

// ActionStarted fires before an action is invoked.
type ActionStarted struct {
	Header
	Action    core.Action `json:"-"`
	StartedAt time.Time   `json:"-"`
}

func (ActionStarted) Kind() string { return "action_started" }

// ActionFinished fires after an action's retry loop terminates.
type ActionFinished struct {
	Header
	Action   core.Action       `json:"-"`
	Status   core.ActionStatus `json:"-"`
	Duration time.Duration     `json:"-"`
	Err      error             `json:"-"`
}

func (ActionFinished) Kind() string { return "action_finished" }

// GoalAchieved fires when the planner returns an empty plan for a non-nil goal.
type GoalAchieved struct {
	Header
	Goal *core.Goal `json:"-"`
}

func (GoalAchieved) Kind() string { return "goal_achieved" }

// InteractionBoundary binds one model/tool protocol event to the exact process
// deployment and logical interaction that produced it.
type InteractionBoundary struct {
	Header
	Deployment    core.DeploymentRef `json:"deployment"`
	InteractionID string             `json:"interaction_id"`
	Boundary      interaction.Event  `json:"boundary"`
}

func (InteractionBoundary) Kind() string { return "interaction_boundary" }

// ModelCallRecorded fires when an LLM call is attributed to a process.
type ModelCallRecorded struct {
	Header
	Call core.ModelCall `json:"-"`
}

func (ModelCallRecorded) Kind() string { return "model_call_recorded" }

// EmbeddingCallRecorded mirrors [ModelCallRecorded] for the embeddings path.
type EmbeddingCallRecorded struct {
	Header
	Call core.EmbeddingCall `json:"-"`
}

func (EmbeddingCallRecorded) Kind() string { return "embedding_call_recorded" }

// envelope is the on-wire JSON shape for every event: a discriminator
// field plus the Header's timestamp / process id plus an opaque
// payload object. Centralized here so each concrete event's MarshalJSON
// is a one-liner.
type envelope struct {
	Kind      string    `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
	ProcessID string    `json:"process_id"`
	Payload   any       `json:"payload,omitempty"`
}

// emit wraps the supplied payload in an envelope, fills the
// discriminator and header fields from e, and marshals to JSON. It's the
// shared body of every event's MarshalJSON.
func emit(e Event, payload any) ([]byte, error) {
	return json.Marshal(envelope{
		Kind:      e.Kind(),
		Timestamp: e.Timestamp(),
		ProcessID: e.ProcessID(),
		Payload:   payload,
	})
}

// errString collapses an error to its message; nil returns "" so the
// JSON encoder can omitempty-elide it.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
