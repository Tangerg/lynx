package runtime

import (
	"context"
	"errors"
	"maps"
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

	// Subtree budget plumbing — children are spawned via
	// Platform.CreateChildProcess; ownCost/ownTokens accumulate via
	// RecordUsage, called by integration code that hears LLM events.
	// Usage() walks children recursively to produce subtree totals.
	children  []*AgentProcess
	ownCost   float64
	ownTokens int

	terminate        chan core.TerminationScopeSignal
	pendingAwaitable atomic.Pointer[awaitSlot]
	toolCallCancel   atomic.Pointer[context.CancelFunc]

	blackboard core.Blackboard
	determiner worldStateDeterminer
	planner    plan.Planner
	system     *plan.PlanningSystem
	platform   *Platform
}

// awaitSlot is the parking spot used by the AwaitInput / ResumeProcess
// pair: AwaitInput stores the awaitable here and flips the process to
// StatusWaiting; ResumeProcess swaps the slot out and routes the
// response through awaitable.OnResponseAny so the user-supplied
// handler runs (typically mutating the blackboard). The single-field
// wrapper exists so atomic.Pointer can park an interface value
// (Go won't let us use atomic.Pointer[core.Awaitable] directly with
// concrete-typed Stores).
type awaitSlot struct {
	awaitable core.Awaitable
}

// ActionInvocation is one row of the per-process history.
type ActionInvocation struct {
	ActionName string
	Timestamp  time.Time
	Duration   time.Duration
	Status     core.ActionStatus
	Attempts   int
}

// newAgentProcess assembles a process from its inputs. Internal — users
// invoke Platform.RunAgent which assembles every dependency.
func newAgentProcess(
	id string,
	agentDef *core.Agent,
	opts *core.ProcessOptions,
	bb core.Blackboard,
	determiner worldStateDeterminer,
	planner plan.Planner,
	system *plan.PlanningSystem,
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
		system:          system,
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

// Usage returns the subtree-aggregated cost / token / action totals.
// Cost and tokens come from [AgentProcess.RecordUsage] calls (zero
// unless integration code wires them up). The action count is the
// recursive sum of every History across this process and all child
// processes spawned via [Platform.CreateChildProcess]. [BudgetPolicy]
// uses the result so a parent's budget governs its entire delegation
// tree.
func (p *AgentProcess) Usage() (cost float64, tokens int, actions int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cost = p.ownCost
	tokens = p.ownTokens
	actions = len(p.history)

	for _, child := range p.children {
		c, t, a := child.Usage()
		cost += c
		tokens += t
		actions += a
	}
	return
}

// RecordUsage adds a single LLM call's cost (USD) and token count to
// this process's running totals. Integration code calls this — typically
// from a listener wired to LLMResponseEvent or an LLM-client adapter
// that knows the per-model rate. The framework itself never invents
// numbers here.
func (p *AgentProcess) RecordUsage(cost float64, tokens int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ownCost += cost
	p.ownTokens += tokens
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

// TerminateToolCall fires the cancel func of the most recently derived
// tool-call context (if any). Action bodies that derive their tool
// invocation contexts via [core.ProcessContext.ToolCallContext] observe
// ctx.Done() at this point and abort. No-op when no tool-call context
// is currently registered.
func (p *AgentProcess) TerminateToolCall(reason string) {
	cancel := p.toolCallCancel.Load()
	if cancel == nil || *cancel == nil {
		return
	}
	(*cancel)()
	_ = reason // reserved for future event publishing
}

// registerToolCallCancel installs a fresh cancel func and returns a
// release closure that detaches it. Used by
// [core.ProcessContext.ToolCallContext]. A new registration replaces any
// previously-stored one (the old context becomes orphaned — its owning
// action body should already be done by the time a new tool call
// starts).
func (p *AgentProcess) registerToolCallCancel(cancel context.CancelFunc) (release func()) {
	cell := &cancel
	p.toolCallCancel.Store(cell)
	return func() {
		// Only clear if we still own the slot — a newer registration
		// would have replaced us, and we mustn't stomp it.
		p.toolCallCancel.CompareAndSwap(cell, nil)
	}
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

// AwaitInput parks the supplied awaitable on the process and returns
// [core.ActionWaiting] so the calling action's typed-action wrapper
// transitions the process to [core.StatusWaiting]. The action's tick
// loop exits at that boundary; the user resumes the process by calling
// [Platform.ResumeProcess], which routes the response through
// awaitable.OnResponseAny — typically mutating the blackboard so the
// next planning tick sees fresh state.
func (p *AgentProcess) AwaitInput(req core.Awaitable) core.ActionStatus {
	if req == nil {
		return core.ActionFailed
	}

	p.pendingAwaitable.Store(&awaitSlot{awaitable: req})
	p.publishEvent(event.ProcessWaitingEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Awaitable: req,
	})
	return core.ActionWaiting
}

// deliverResponse is the runtime's hook for resuming a waiting process.
// It atomically claims the parked slot and forwards the response to the
// awaitable's typed handler via [core.Awaitable.OnResponseAny]. Returns
// the handler's [core.ResponseImpact] (so [Platform.ResumeProcess] can
// surface it to the caller — Updated typically means the caller should
// re-run the process to pick up the new state).
//
// Returns an error when no slot is parked or the response value doesn't
// match the awaitable's expected type.
func (p *AgentProcess) deliverResponse(response any) (core.ResponseImpact, error) {
	slot := p.pendingAwaitable.Swap(nil)
	if slot == nil {
		return core.ResponseImpactUnchanged, errors.New("no awaitable pending")
	}
	return slot.awaitable.OnResponseAny(response)
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
	return maps.Clone(p.excludedActions)
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
