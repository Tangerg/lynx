// Package event defines the framework's lifecycle event types and the
// multicast Listener that ferries them to subscribers. Events are
// type-erased to "any" by the runtime when published so core can stay
// independent of this package; type-asserting listeners switch on the
// concrete struct.
package event

import (
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

// Event is the common interface — every concrete event embeds BaseEvent
// so it satisfies these methods without each type re-implementing them.
type Event interface {
	Timestamp() time.Time
	ProcessID() string
	EventName() string
}

// BaseEvent is the embedded carrier shared across all concrete events.
type BaseEvent struct {
	At  time.Time
	PID string
}

func (b BaseEvent) Timestamp() time.Time { return b.At }
func (b BaseEvent) ProcessID() string    { return b.PID }
func (b BaseEvent) EventName() string    { return "base" }

// NewBaseEvent stamps a fresh event with the configured time source.
func NewBaseEvent(processID string) BaseEvent {
	return BaseEvent{At: core.Now(), PID: processID}
}

// --- Platform-level events ------------------------------------------------

type AgentDeployedEvent struct {
	BaseEvent
	AgentName string
}

func (AgentDeployedEvent) EventName() string { return "agent_deployed" }

type AgentUndeployedEvent struct {
	BaseEvent
	AgentName string
}

func (AgentUndeployedEvent) EventName() string { return "agent_undeployed" }

// --- Process lifecycle ----------------------------------------------------

type ProcessCreatedEvent struct {
	BaseEvent
	Bindings map[string]any
}

func (ProcessCreatedEvent) EventName() string { return "process_created" }

type ProcessCompletedEvent struct {
	BaseEvent
	Goal *core.Goal
}

func (ProcessCompletedEvent) EventName() string { return "process_completed" }

type ProcessFailedEvent struct {
	BaseEvent
	Err error
}

func (ProcessFailedEvent) EventName() string { return "process_failed" }

type ProcessStuckEvent struct {
	BaseEvent
	LastWorld core.WorldState
}

func (ProcessStuckEvent) EventName() string { return "process_stuck" }

type ProcessWaitingEvent struct {
	BaseEvent
	Awaitable core.Awaitable
}

func (ProcessWaitingEvent) EventName() string { return "process_waiting" }

type ProcessPausedEvent struct {
	BaseEvent
	Reason string
}

func (ProcessPausedEvent) EventName() string { return "process_paused" }

type ProcessKilledEvent struct {
	BaseEvent
	Reason string
}

func (ProcessKilledEvent) EventName() string { return "process_killed" }

type ProcessTerminatedEvent struct {
	BaseEvent
	Reason string
	Scope  core.TerminationScope
}

func (ProcessTerminatedEvent) EventName() string { return "process_terminated" }

// --- Planning -------------------------------------------------------------

type ReadyToPlanEvent struct {
	BaseEvent
	World core.WorldState
}

func (ReadyToPlanEvent) EventName() string { return "ready_to_plan" }

type PlanFormulatedEvent struct {
	BaseEvent
	Plan *plan.Plan
}

func (PlanFormulatedEvent) EventName() string { return "plan_formulated" }

type ReplanRequestedEvent struct {
	BaseEvent
	Action string
	Reason string
}

func (ReplanRequestedEvent) EventName() string { return "replan_requested" }

// --- Execution ------------------------------------------------------------

type ActionExecutionStartEvent struct {
	BaseEvent
	Action    core.Action
	StartedAt time.Time
}

func (ActionExecutionStartEvent) EventName() string { return "action_execution_start" }

type ActionExecutionResultEvent struct {
	BaseEvent
	Action   core.Action
	Status   core.ActionStatus
	Duration time.Duration
	Err      error
}

func (ActionExecutionResultEvent) EventName() string { return "action_execution_result" }

type ObjectBoundEvent struct {
	BaseEvent
	Key  string
	Type string
}

func (ObjectBoundEvent) EventName() string { return "object_bound" }

type GoalAchievedEvent struct {
	BaseEvent
	Goal *core.Goal
}

func (GoalAchievedEvent) EventName() string { return "goal_achieved" }

// --- LLM / RAG (best-effort tracking; emitted only when integration layer
//     supplies the metrics) -------------------------------------------------

type LLMRequestEvent struct {
	BaseEvent
	Model    string
	Provider string
	Prompt   string
}

func (LLMRequestEvent) EventName() string { return "llm_request" }

type LLMResponseEvent struct {
	BaseEvent
	Model        string
	InputTokens  int
	OutputTokens int
	Duration     time.Duration
	Err          error
}

func (LLMResponseEvent) EventName() string { return "llm_response" }

// --- Listener / multicast -------------------------------------------------

// Listener is the subscriber surface. Implementations should be
// non-blocking; the multicast holds an RLock while delivering.
type Listener interface {
	OnEvent(e Event)
}

// ListenerFunc adapts a plain function into Listener.
type ListenerFunc func(e Event)

func (f ListenerFunc) OnEvent(e Event) { f(e) }

// Multicast is the concurrent-safe fan-out. Add/Remove may run while
// OnEvent is delivering (writer-lock blocks until current delivery
// finishes).
type Multicast struct {
	mu        sync.RWMutex
	listeners []Listener
}

// NewMulticast returns an empty Multicast.
func NewMulticast() *Multicast { return &Multicast{} }

// Add appends a listener. Nil listeners are ignored to keep callers from
// having to nil-check.
func (m *Multicast) Add(l Listener) {
	if l == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, l)
}

// Remove drops the supplied listener (by pointer identity). Listeners not
// present are silently ignored.
func (m *Multicast) Remove(l Listener) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, existing := range m.listeners {
		if existing == l {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			return
		}
	}
}

// OnEvent delivers to every registered listener, isolating each call so a
// panicking listener doesn't take down the rest.
func (m *Multicast) OnEvent(e Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, listener := range m.listeners {
		safeDeliver(listener, e)
	}
}

// safeDeliver invokes the listener with a panic guard. Panicking
// listeners are a bug, but we don't want one to take down the whole
// process — production deployments can wire a recovering listener that
// reports to logs / metrics.
func safeDeliver(l Listener, e Event) {
	defer func() { _ = recover() }()
	l.OnEvent(e)
}
