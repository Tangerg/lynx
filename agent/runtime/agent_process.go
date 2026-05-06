package runtime

import (
	"context"
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
// Internal layout — three concerns embedded as sub-structs so related
// fields & methods cluster together:
//
//   - processState   mu-protected status / goal / history / failure /
//                    exclusions. Owns the main mutex; processBudget
//                    shares it via a pointer.
//   - processBudget  subtree cost / token / action aggregation; mu
//                    pointer points at processState.mu.
//   - processSignals channel + atomic-based signalling primitives
//                    (terminate / awaitable slot / toolCallCancel) —
//                    no shared lock, all built on lock-free primitives.
//
// The remaining top-level fields are construction-time wiring (id /
// agent / options / blackboard / determiner / planner / system /
// platform) — immutable after newAgentProcess returns.
type AgentProcess struct {
	id        string
	parentID  string
	agent     *core.Agent
	options   *core.ProcessOptions
	startedAt time.Time

	processState   // mu + status + goal + lastWorld + history + failure + exclusions
	processBudget  // children + ownCost + ownTokens (mu shared with processState)
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
		id:             id,
		agent:          agentDef,
		options:        opts,
		startedAt:      core.Now(),
		processState:   newProcessState(),
		processSignals: newProcessSignals(),
		blackboard:     bb,
		determiner:     determiner,
		planner:        planner,
		system:         system,
		platform:       platform,
	}
	p.processBudget.lock = &p.processState.mu // budget shares state's mutex
	return p
}

// --- core.Process read surface --------------------------------------------
//
// Status / Goal / LastWorldState / Failure / History come from the embedded
// processState; the immutable identity getters live here.

func (p *AgentProcess) ID() string                    { return p.id }
func (p *AgentProcess) ParentID() string              { return p.parentID }
func (p *AgentProcess) StartedAt() time.Time          { return p.startedAt }
func (p *AgentProcess) Blackboard() core.Blackboard   { return p.blackboard }
func (p *AgentProcess) Options() *core.ProcessOptions { return p.options }
func (p *AgentProcess) AgentDef() *core.Agent         { return p.agent }

// Usage returns the subtree-aggregated cost / token / action totals.
// Cost and tokens come from [AgentProcess.RecordUsage] calls (zero
// unless integration code wires them up). The action count is the
// recursive sum of every History across this process and all child
// processes spawned via [Platform.CreateChildProcess]. [core.BudgetPolicy]
// uses the result so a parent's budget governs its entire delegation
// tree.
func (p *AgentProcess) Usage() (cost float64, tokens int, actions int) {
	// Snapshot history length under the shared mutex, then delegate to
	// the embedded processBudget which walks children.
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

// --- event plumbing -------------------------------------------------------

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

// --- tracing helpers ------------------------------------------------------

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
