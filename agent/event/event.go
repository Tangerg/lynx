// Package event defines the framework's lifecycle event types and the
// multicast Listener that ferries them to subscribers. Events are
// type-erased to "any" by the runtime when published so core can stay
// independent of this package; type-asserting listeners switch on the
// concrete struct.
//
// Every event type implements [encoding/json.Marshaler] and produces a
// self-describing JSON object — useful for audit logs, federation, and
// observability sinks that want raw payloads. Marshaling is one-way:
// interface-typed fields ([core.Action], [core.WorldState],
// [core.Awaitable], [error]) collapse to lossy summary forms (a name
// string, a state map, …). Round-trip deserialization is intentionally
// not provided — listeners that need it should consume events in their
// in-memory form.
package event

import (
	"encoding/json"
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
// Field names use JSON-friendly tags so each event's marshaler can drop
// `At` / `PID` straight into the envelope.
type BaseEvent struct {
	At  time.Time `json:"timestamp"`
	PID string    `json:"process_id"`
}

func (b BaseEvent) Timestamp() time.Time { return b.At }
func (b BaseEvent) ProcessID() string    { return b.PID }
func (b BaseEvent) EventName() string    { return "base" }

// NewBaseEvent stamps a fresh event with the configured time source.
func NewBaseEvent(processID string) BaseEvent {
	return BaseEvent{At: core.Now(), PID: processID}
}

// envelope is the on-wire JSON shape for every event: a discriminator
// field plus the BaseEvent's timestamp / process id plus an opaque
// payload object. Centralised here so each concrete event's MarshalJSON
// is a one-liner.
type envelope struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	ProcessID string    `json:"process_id"`
	Payload   any       `json:"payload,omitempty"`
}

// emit wraps the supplied payload in an envelope, fills the
// discriminator and base fields from e, and marshals to JSON. It's the
// shared body of every event's MarshalJSON.
func emit(e Event, payload any) ([]byte, error) {
	return json.Marshal(envelope{
		Event:     e.EventName(),
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

// goalSummary is the wire shape for a *core.Goal — lossy on the
// non-serializable fields ([core.IoBinding].Type already strings, but
// [core.CostFunc] callbacks can't round-trip).
type goalSummary struct {
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Pre         []string         `json:"pre,omitempty"`
	Inputs      []core.IoBinding `json:"inputs,omitempty"`
	OutputType  string           `json:"output_type,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Examples    []string         `json:"examples,omitempty"`
}

func summarizeGoal(g *core.Goal) *goalSummary {
	if g == nil {
		return nil
	}
	out := &goalSummary{
		Name:        g.Name,
		Description: g.Description,
		Pre:         g.Pre,
		Inputs:      g.Inputs,
		Tags:        g.Tags,
		Examples:    g.Examples,
	}
	if g.OutputType != nil {
		out.OutputType = *g.OutputType
	}
	return out
}

// actionName returns the action's name, or "" when nil.
func actionName(a core.Action) string {
	if a == nil {
		return ""
	}
	return a.Metadata().Name
}

// actionNames maps the action slice to its names. Used by plan summaries.
func actionNames(actions []core.Action) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, actionName(a))
	}
	return out
}

// planSummary is the wire shape for *plan.Plan — actions reduce to the
// ordered list of their names, goal becomes a goalSummary.
type planSummary struct {
	Actions []string     `json:"actions,omitempty"`
	Goal    *goalSummary `json:"goal,omitempty"`
}

func summarizePlan(p *plan.Plan) *planSummary {
	if p == nil {
		return nil
	}
	return &planSummary{
		Actions: actionNames(p.Actions),
		Goal:    summarizeGoal(p.Goal),
	}
}

// worldSnapshot captures everything serializable from a [core.WorldState]:
// the condition map and the snapshot timestamp.
type worldSnapshot struct {
	State     map[string]core.Determination `json:"state,omitempty"`
	Timestamp time.Time                     `json:"timestamp,omitempty"`
}

func snapshotWorld(ws core.WorldState) *worldSnapshot {
	if ws == nil {
		return nil
	}
	return &worldSnapshot{State: ws.State(), Timestamp: ws.Timestamp()}
}

// awaitableSummary is the wire shape for [core.Awaitable]: the stable id
// and the (untyped) prompt payload. Concrete prompt types serialize via
// their own MarshalJSON / struct tags; opaque payloads end up as the
// closest JSON form encoding/json can produce.
type awaitableSummary struct {
	ID     string `json:"id,omitempty"`
	Prompt any    `json:"prompt,omitempty"`
}

func summarizeAwaitable(a core.Awaitable) *awaitableSummary {
	if a == nil {
		return nil
	}
	return &awaitableSummary{ID: a.ID(), Prompt: a.PromptAny()}
}

// --- Platform-level events ------------------------------------------------

type AgentDeployedEvent struct {
	BaseEvent
	AgentName string `json:"agent_name"`
}

func (AgentDeployedEvent) EventName() string { return "agent_deployed" }

func (e AgentDeployedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
}

type AgentUndeployedEvent struct {
	BaseEvent
	AgentName string `json:"agent_name"`
}

func (AgentUndeployedEvent) EventName() string { return "agent_undeployed" }

func (e AgentUndeployedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
}

// --- Process lifecycle ----------------------------------------------------

type ProcessCreatedEvent struct {
	BaseEvent
	Bindings map[string]any `json:"bindings,omitempty"`
}

func (ProcessCreatedEvent) EventName() string { return "process_created" }

func (e ProcessCreatedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"bindings": e.Bindings})
}

type ProcessCompletedEvent struct {
	BaseEvent
	Goal *core.Goal `json:"-"`
}

func (ProcessCompletedEvent) EventName() string { return "process_completed" }

func (e ProcessCompletedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"goal": summarizeGoal(e.Goal)})
}

type ProcessFailedEvent struct {
	BaseEvent
	Err error `json:"-"`
}

func (ProcessFailedEvent) EventName() string { return "process_failed" }

func (e ProcessFailedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"error": errString(e.Err)})
}

type ProcessStuckEvent struct {
	BaseEvent
	LastWorld core.WorldState `json:"-"`
}

func (ProcessStuckEvent) EventName() string { return "process_stuck" }

func (e ProcessStuckEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"world": snapshotWorld(e.LastWorld)})
}

type ProcessWaitingEvent struct {
	BaseEvent
	Awaitable core.Awaitable `json:"-"`
}

func (ProcessWaitingEvent) EventName() string { return "process_waiting" }

func (e ProcessWaitingEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"awaitable": summarizeAwaitable(e.Awaitable)})
}

type ProcessPausedEvent struct {
	BaseEvent
	Reason string `json:"reason,omitempty"`
}

func (ProcessPausedEvent) EventName() string { return "process_paused" }

func (e ProcessPausedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason})
}

type ProcessKilledEvent struct {
	BaseEvent
	Reason string `json:"reason,omitempty"`
}

func (ProcessKilledEvent) EventName() string { return "process_killed" }

func (e ProcessKilledEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason})
}

type ProcessTerminatedEvent struct {
	BaseEvent
	Reason string                `json:"reason,omitempty"`
	Scope  core.TerminationScope `json:"-"`
}

func (ProcessTerminatedEvent) EventName() string { return "process_terminated" }

func (e ProcessTerminatedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason, "scope": e.Scope.String()})
}

// --- Planning -------------------------------------------------------------

type ReadyToPlanEvent struct {
	BaseEvent
	World core.WorldState `json:"-"`
}

func (ReadyToPlanEvent) EventName() string { return "ready_to_plan" }

func (e ReadyToPlanEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"world": snapshotWorld(e.World)})
}

type PlanFormulatedEvent struct {
	BaseEvent
	Plan *plan.Plan `json:"-"`
}

func (PlanFormulatedEvent) EventName() string { return "plan_formulated" }

func (e PlanFormulatedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"plan": summarizePlan(e.Plan)})
}

type ReplanRequestedEvent struct {
	BaseEvent
	Action string `json:"action,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (ReplanRequestedEvent) EventName() string { return "replan_requested" }

func (e ReplanRequestedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": e.Action, "reason": e.Reason})
}

// --- Execution ------------------------------------------------------------

type ActionExecutionStartEvent struct {
	BaseEvent
	Action    core.Action `json:"-"`
	StartedAt time.Time   `json:"-"`
}

func (ActionExecutionStartEvent) EventName() string { return "action_execution_start" }

func (e ActionExecutionStartEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": actionName(e.Action), "started_at": e.StartedAt})
}

type ActionExecutionResultEvent struct {
	BaseEvent
	Action   core.Action       `json:"-"`
	Status   core.ActionStatus `json:"-"`
	Duration time.Duration     `json:"-"`
	Err      error             `json:"-"`
}

func (ActionExecutionResultEvent) EventName() string { return "action_execution_result" }

func (e ActionExecutionResultEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"action":      actionName(e.Action),
		"status":      e.Status.String(),
		"duration_ns": e.Duration.Nanoseconds(),
		"error":       errString(e.Err),
	})
}

type ObjectBoundEvent struct {
	BaseEvent
	Key  string `json:"key,omitempty"`
	Type string `json:"type,omitempty"`
}

func (ObjectBoundEvent) EventName() string { return "object_bound" }

func (e ObjectBoundEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"key": e.Key, "type": e.Type})
}

type GoalAchievedEvent struct {
	BaseEvent
	Goal *core.Goal `json:"-"`
}

func (GoalAchievedEvent) EventName() string { return "goal_achieved" }

func (e GoalAchievedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"goal": summarizeGoal(e.Goal)})
}

// --- LLM / RAG (best-effort tracking; emitted only when integration layer
//     supplies the metrics) -------------------------------------------------

type LLMRequestEvent struct {
	BaseEvent
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

func (LLMRequestEvent) EventName() string { return "llm_request" }

func (e LLMRequestEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"model":    e.Model,
		"provider": e.Provider,
		"prompt":   e.Prompt,
	})
}

type LLMResponseEvent struct {
	BaseEvent
	Model        string        `json:"model,omitempty"`
	InputTokens  int           `json:"input_tokens,omitempty"`
	OutputTokens int           `json:"output_tokens,omitempty"`
	Duration     time.Duration `json:"-"`
	Err          error         `json:"-"`
}

func (LLMResponseEvent) EventName() string { return "llm_response" }

func (e LLMResponseEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"model":         e.Model,
		"input_tokens":  e.InputTokens,
		"output_tokens": e.OutputTokens,
		"duration_ns":   e.Duration.Nanoseconds(),
		"error":         errString(e.Err),
	})
}

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
// panicking listener doesn't take down the rest. Listeners are snapshotted
// under the lock and then invoked outside it, so a slow listener can't
// block concurrent Add / Remove calls.
func (m *Multicast) OnEvent(e Event) {
	m.mu.RLock()
	listeners := make([]Listener, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()

	for _, listener := range listeners {
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
