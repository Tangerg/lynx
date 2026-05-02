package runtime

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/plan"
)

// AgentProcess is the runtime's mutable per-execution state. It implements
// core.Process (read surface) plus mutation methods (TerminateAgent,
// AwaitInput, ...) that only the runtime should call.
type AgentProcess struct {
	id        string
	parentID  string
	agent     *core.Agent
	options   *core.ProcessOptions
	startedAt time.Time

	mu              sync.RWMutex
	status          core.AgentProcessStatus
	goal            *core.Goal
	lastWorld       core.WorldState
	history         []ActionInvocation
	failure         error
	excludedActions map[string]struct{}

	terminate        chan core.TerminationScopeSignal
	pendingAwaitable atomic.Pointer[awaitSlot]

	blackboard core.Blackboard
	determiner WorldStateDeterminer
	planner    plan.Planner
	platform   *Platform
}

// awaitSlot is the parking spot used by the AwaitInput / ResumeProcess
// pair. The action publishes a request here; the platform's resume API
// delivers the response.
type awaitSlot struct {
	awaitable core.Awaitable
	respond   chan any
}

// ActionInvocation is one row of the per-process history.
type ActionInvocation struct {
	ActionName string
	Timestamp  time.Time
	Duration   time.Duration
	Status     core.ActionStatus
	Attempts   int
}

// NewAgentProcess assembles a process from its inputs. The runtime calls
// this; users invoke Platform.RunAgent.
func NewAgentProcess(
	id string,
	agentDef *core.Agent,
	opts *core.ProcessOptions,
	bb core.Blackboard,
	determiner WorldStateDeterminer,
	planner plan.Planner,
	platform *Platform,
) *AgentProcess {
	return &AgentProcess{
		id:              id,
		agent:           agentDef,
		options:         opts,
		startedAt:       core.Now(),
		status:          core.StatusNotStarted,
		excludedActions: map[string]struct{}{},
		terminate:       make(chan core.TerminationScopeSignal, 1),
		blackboard:      bb,
		determiner:      determiner,
		planner:         planner,
		platform:        platform,
	}
}

// --- core.Process read surface --------------------------------------------

func (p *AgentProcess) ID() string              { return p.id }
func (p *AgentProcess) ParentID() string        { return p.parentID }
func (p *AgentProcess) StartedAt() time.Time    { return p.startedAt }
func (p *AgentProcess) Blackboard() core.Blackboard   { return p.blackboard }
func (p *AgentProcess) Options() *core.ProcessOptions { return p.options }
func (p *AgentProcess) AgentDef() *core.Agent         { return p.agent }

func (p *AgentProcess) Status() core.AgentProcessStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func (p *AgentProcess) Goal() *core.Goal {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.goal
}

func (p *AgentProcess) LastWorldState() core.WorldState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastWorld
}

func (p *AgentProcess) Failure() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.failure
}

// History returns a snapshot of completed action invocations.
func (p *AgentProcess) History() []ActionInvocation {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return slices.Clone(p.history)
}

// HistorySize is the cheap form for the early-termination policy.
func (p *AgentProcess) HistorySize() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.history)
}

// Usage returns a placeholder cost/token/action snapshot. The framework
// doesn't yet track LLM token cost; the slot exists so Budget enforcement
// can be added later.
func (p *AgentProcess) Usage() (cost float64, tokens int, actions int) {
	return 0, 0, p.HistorySize()
}

// --- core.Process mutation surface ----------------------------------------

// TerminateAgent queues a "stop the whole process" signal. Tick consumes
// the channel at the next boundary.
func (p *AgentProcess) TerminateAgent(reason string) {
	p.queueTermination(core.TerminationScopeAgent, reason)
}

// TerminateAction queues a "skip the current action and re-plan" signal.
func (p *AgentProcess) TerminateAction(reason string) {
	p.queueTermination(core.TerminationScopeAction, reason)
}

func (p *AgentProcess) queueTermination(scope core.TerminationScope, reason string) {
	signal := core.TerminationScopeSignal{Scope: scope, Reason: reason}
	select {
	case p.terminate <- signal:
	default:
		// A signal is already pending — drop the duplicate; the existing
		// signal will be picked up at the next tick boundary.
	}
}

// AwaitInput parks the calling action until the platform delivers a
// response. Returns ActionWaiting so the runtime knows to flip the process
// to StatusWaiting.
func (p *AgentProcess) AwaitInput(req core.Awaitable) core.ActionStatus {
	if req == nil {
		return core.ActionFailed
	}

	slot := &awaitSlot{awaitable: req, respond: make(chan any, 1)}
	p.pendingAwaitable.Store(slot)
	p.publishEvent(event.ProcessWaitingEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Awaitable: req,
	})
	return core.ActionWaiting
}

// PendingAwaitable returns the parked Awaitable (if any) — the platform's
// status API surfaces this.
func (p *AgentProcess) PendingAwaitable() core.Awaitable {
	slot := p.pendingAwaitable.Load()
	if slot == nil {
		return nil
	}
	return slot.awaitable
}

// DeliverResponse is the runtime's hook for resuming a waiting process.
// Returns true if a slot was waiting; false otherwise.
func (p *AgentProcess) DeliverResponse(response any) bool {
	slot := p.pendingAwaitable.Swap(nil)
	if slot == nil {
		return false
	}

	select {
	case slot.respond <- response:
		return true
	default:
		return false
	}
}

// --- internal mutators (used by tick / executeAction) ---------------------

func (p *AgentProcess) setStatus(s core.AgentProcessStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
}

func (p *AgentProcess) setGoal(g *core.Goal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.goal = g
}

func (p *AgentProcess) setLastWorld(ws core.WorldState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastWorld = ws
}

func (p *AgentProcess) setFailure(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failure = err
}

func (p *AgentProcess) recordInvocation(inv ActionInvocation) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.history = append(p.history, inv)
}

func (p *AgentProcess) excludeAction(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.excludedActions[name] = struct{}{}
}

func (p *AgentProcess) clearExclusions() {
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.excludedActions)
}

func (p *AgentProcess) snapshotExclusions() map[string]struct{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make(map[string]struct{}, len(p.excludedActions))
	for name := range p.excludedActions {
		out[name] = struct{}{}
	}
	return out
}

// publishEvent dispatches via the platform's multicast listener (if wired).
func (p *AgentProcess) publishEvent(e event.Event) {
	if p.platform == nil {
		return
	}
	p.platform.publish(e)
}

// publishAny accepts the type-erased event used by ProcessContext.Publish.
func (p *AgentProcess) publishAny(e any) {
	ev, ok := e.(event.Event)
	if !ok {
		return
	}
	p.publishEvent(ev)
}

// makeRunning is the idempotent transition from NotStarted to Running. It
// returns true on the first transition (so the caller knows to start the
// loop) and false thereafter.
func (p *AgentProcess) makeRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status != core.StatusNotStarted {
		return false
	}
	p.status = core.StatusRunning
	return true
}

// drainTerminate pulls a signal off the channel without blocking. nil
// result means no signal pending.
func (p *AgentProcess) drainTerminate() *core.TerminationScopeSignal {
	select {
	case sig := <-p.terminate:
		return &sig
	default:
		return nil
	}
}

// startTickSpan creates a span scoped to one tick.
func (p *AgentProcess) startTickSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return core.AgentTracer().Start(ctx, name,
		trace.WithAttributes(
			attribute.String("lynx.agent.name", p.agent.Name),
			attribute.String("lynx.agent.process_id", p.id),
		),
	)
}

// finishSpanWithError records err on span and sets the OTel error status.
func finishSpanWithError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
