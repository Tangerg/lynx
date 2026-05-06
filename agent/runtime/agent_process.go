package runtime

import (
	"context"
	"maps"
	"slices"
	"sync"
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
//
// Internal layout: identity / wiring fields sit on the struct directly;
// channel + atomic-based signalling (terminate / pendingAwaitable /
// toolCallCancel) is encapsulated in the embedded processSignals so
// related primitives stay together.
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

	processBudget  // children + ownCost + ownTokens (mu shared with AgentProcess)
	processSignals // terminate channel + awaitable slot + toolCallCancel

	blackboard core.Blackboard
	determiner worldStateDeterminer
	planner    plan.Planner
	system     *plan.PlanningSystem
	platform   *Platform
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
	p := &AgentProcess{
		id:              id,
		agent:           agentDef,
		options:         opts,
		startedAt:       core.Now(),
		status:          core.StatusNotStarted,
		excludedActions: map[string]struct{}{},
		processSignals:  newProcessSignals(),
		blackboard:      bb,
		determiner:      determiner,
		planner:         planner,
		system:          system,
		platform:        platform,
	}
	p.processBudget.mu = &p.mu // share mutex — budget writes live alongside state writes
	return p
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
// processes spawned via [Platform.CreateChildProcess]. [core.BudgetPolicy]
// uses the result so a parent's budget governs its entire delegation
// tree.
func (p *AgentProcess) Usage() (cost float64, tokens int, actions int) {
	// Snapshot history length under main mu, then delegate to the
	// embedded processBudget which walks children.
	p.mu.RLock()
	ownActions := len(p.history)
	p.mu.RUnlock()
	return p.usage(ownActions)
}

// RecordUsage adds a single LLM call's cost (USD) and token count to
// this process's running totals. Integration code calls this from an
// LLM-client adapter that knows the per-model rate. The framework
// itself never invents numbers here.
func (p *AgentProcess) RecordUsage(cost float64, tokens int) {
	p.recordUsage(cost, tokens)
}

// --- core.Process mutation surface ----------------------------------------
//
// Terminate*, AwaitInput, and the internal helpers (registerToolCallCancel,
// deliverResponse, drainTerminate) are inherited via the embedded
// processSignals; the methods below add the AgentProcess-specific bits
// (events, scope wiring) on top.

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
	p.fireToolCallCancel()
	_ = reason // reserved for future event publishing
}

// AwaitInput parks the supplied awaitable on the process and returns
// [core.ActionWaiting] so the calling action's typed-action wrapper
// transitions the process to [core.StatusWaiting]. The action's tick
// loop exits at that boundary; the user resumes the process by calling
// [Platform.ResumeProcess], which routes the response through
// awaitable.OnResponseAny — typically mutating the blackboard so the
// next planning tick sees fresh state.
func (p *AgentProcess) AwaitInput(req core.Awaitable) core.ActionStatus {
	status := p.parkAwaitable(req)
	if status == core.ActionWaiting {
		p.publishEvent(p.buildWaitingEvent(p.id, req))
	}
	return status
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
