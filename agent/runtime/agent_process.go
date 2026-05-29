package runtime

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/core/model/chat"
)

// AgentProcess is the runtime's mutable per-execution state. It implements
// core.Process (read surface) plus mutation methods (TerminateAgent,
// AwaitInput, ...) that only the runtime should call.
//
// Internal layout — three concerns kept as named sub-struct fields so
// related fields & methods cluster together while the access path stays
// explicit at every call site:
//
//   - state    mu-protected status / goal / history / failure /
//     exclusions. Owns the main mutex; budget shares it via
//     a pointer.
//   - budget   subtree cost / token / action aggregation; lock pointer
//     points at state.mu.
//   - signals  channel + atomic-based signaling primitives
//     (terminate / awaitable slot / toolCallCancel) — no
//     shared lock, all built on lock-free primitives.
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

	state   processState
	budget  processBudget
	signals processSignals

	blackboard core.Blackboard
	determiner *blackboardDeterminer
	planner    planning.Planner
	system     *planning.System
	platform   *Platform

	// processEvents is the per-process multicast populated from
	// EventListener extensions on ProcessOptions.Extensions. Wired by
	// wireRuntimeDeps on every construction path (createProcess +
	// RestoreProcess); publishEvent still nil-guards it for safety.
	processEvents *event.Multicast
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
// invoke Platform.RunAgent which assembles every dependency. The
// determiner and processEvents are populated by the caller after
// construction because both need the *AgentProcess pointer (the
// determiner wires it as the [core.Process] for user conditions; the
// multicast subscribes to per-process EventListener extensions).
func newAgentProcess(
	id string,
	agentDef *core.Agent,
	options *core.ProcessOptions,
	blackboard core.Blackboard,
	planner planning.Planner,
	system *planning.System,
	platform *Platform,
) *AgentProcess {
	p := &AgentProcess{
		id:         id,
		agent:      agentDef,
		options:    options,
		startedAt:  core.Now(),
		state:      newProcessState(),
		signals:    newProcessSignals(),
		blackboard: blackboard,
		planner:    planner,
		system:     system,
		platform:   platform,
	}
	p.budget.lock = &p.state.mu // budget shares state's mutex
	return p
}

// wireRuntimeDeps finishes the parts of construction that need the
// *AgentProcess pointer itself: the determiner (which wires the process
// as the [core.Process] user-defined conditions evaluate against) and
// the per-process event multicast (subscribing process-scope
// EventListener extensions). Split out of newAgentProcess because both
// fields close over the assembled pointer, and shared by every path
// that builds a process — createProcess for fresh runs, RestoreProcess
// for snapshots re-entering the tick loop. A restored process that
// skips this panics on its first observe (nil determiner).
func (p *AgentProcess) wireRuntimeDeps(extensions []core.Extension) {
	p.determiner = newBlackboardDeterminer(p.system, p.blackboard, p)
	p.processEvents = event.NewMulticast()
	addEventListenerExtensions(p.processEvents, extensions)
}

// --- core.Process read surface --------------------------------------------

func (p *AgentProcess) ID() string                    { return p.id }
func (p *AgentProcess) ParentID() string              { return p.parentID }
func (p *AgentProcess) StartedAt() time.Time          { return p.startedAt }
func (p *AgentProcess) Blackboard() core.Blackboard   { return p.blackboard }
func (p *AgentProcess) Options() *core.ProcessOptions { return p.options }

// Status / Goal / LastWorldState / Failure / History delegate to the
// state sub-struct, which owns the lock.
func (p *AgentProcess) Status() core.AgentProcessStatus { return p.state.getStatus() }
func (p *AgentProcess) Goal() *core.Goal                { return p.state.getGoal() }
func (p *AgentProcess) LastWorldState() core.WorldState { return p.state.getLastWorld() }
func (p *AgentProcess) Failure() error                  { return p.state.getFailure() }
func (p *AgentProcess) History() []ActionInvocation     { return p.state.getHistory() }

// Usage returns the subtree-aggregated cost / token / action totals.
// Cost and tokens come from [AgentProcess.RecordUsage] calls (zero
// unless integration code wires them up). The action count is the
// recursive sum of every History across this process and all child
// processes spawned via [Platform.CreateChildProcess]. [core.BudgetPolicy]
// uses the result so a parent's budget governs its entire delegation
// tree.
func (p *AgentProcess) Usage() (cost float64, tokens int, actions int) {
	return p.budget.usage(p.state.historyLen())
}

// RecordUsage adds a single LLM call's cost (USD) and token count to
// this process's running totals. Integration code calls this from an
// LLM-client adapter that knows the per-model rate. The framework
// itself never invents numbers here.
func (p *AgentProcess) RecordUsage(cost float64, tokens int) {
	p.RecordLLMInvocation(core.LLMInvocation{Cost: cost, PromptTokens: int64(tokens)})
}

// RecordLLMInvocation appends a fully-attributed LLM call to this
// process's history. Integration code calls this once per LLM
// response with the model id, provider, cost, and token breakdown.
// It also publishes an [event.LLMInvocationRecorded] so listeners can
// audit per-call cost/tokens off the event stream.
func (p *AgentProcess) RecordLLMInvocation(inv core.LLMInvocation) {
	if inv.Timestamp.IsZero() {
		inv.Timestamp = core.Now()
	}
	p.budget.recordLLMInvocation(inv)
	p.publishEvent(event.LLMInvocationRecorded{
		BaseEvent:  p.baseEvent(),
		Invocation: inv,
	})
}

// RecordEmbeddingInvocation appends a fully-attributed embedding
// call. Mirrors RecordLLMInvocation for the embeddings path, including
// the [event.EmbeddingInvocationRecorded] publish.
func (p *AgentProcess) RecordEmbeddingInvocation(inv core.EmbeddingInvocation) {
	if inv.Timestamp.IsZero() {
		inv.Timestamp = core.Now()
	}
	p.budget.recordEmbeddingInvocation(inv)
	p.publishEvent(event.EmbeddingInvocationRecorded{
		BaseEvent:  p.baseEvent(),
		Invocation: inv,
	})
}

// LLMInvocations returns the subtree-aggregated LLM invocation
// history in chronological-per-process order.
func (p *AgentProcess) LLMInvocations() []core.LLMInvocation {
	return p.budget.llmHistory()
}

// EmbeddingInvocations returns the subtree-aggregated embedding
// invocation history.
func (p *AgentProcess) EmbeddingInvocations() []core.EmbeddingInvocation {
	return p.budget.embeddingHistory()
}

// --- core.Process mutation surface ----------------------------------------

// TerminateAgent queues a "stop the whole process" signal. Tick consumes
// the channel at the next boundary.
func (p *AgentProcess) TerminateAgent(reason string) {
	p.signals.queueTermination(core.TerminationScopeAgent, reason)
}

// TerminateAction queues a "skip the current action and re-plan" signal.
func (p *AgentProcess) TerminateAction(reason string) {
	p.signals.queueTermination(core.TerminationScopeAction, reason)
}

// TerminateToolCall fires the cancel func of the most recently derived
// tool-call context (if any). Action bodies that derive their tool
// invocation contexts via [core.ProcessContext.ToolCallContext] observe
// ctx.Done() at this point and abort. No-op when no tool-call context
// is currently registered.
func (p *AgentProcess) TerminateToolCall(reason string) {
	p.signals.fireToolCallCancel()
	_ = reason // reserved for future event publishing
}

// PendingAwaitable returns the awaitable currently parked by an
// in-flight [AgentProcess.AwaitInput], or nil when nothing is parked.
// Used by supervisor patterns that want to surface a suspended
// child's pending request back to the parent LLM as tool-result text
// rather than failing the parent.
func (p *AgentProcess) PendingAwaitable() core.Awaitable {
	return p.signals.peekAwaitable()
}

// AwaitInput parks the supplied awaitable on the process and returns
// [core.ActionWaiting] so the calling action's typed-action wrapper
// transitions the process to [core.StatusWaiting]. The action's tick
// loop exits at that boundary; the user resumes the process by calling
// [Platform.ResumeProcess], which routes the response through
// awaitable.OnResponseAny — typically mutating the blackboard so the
// next planning tick sees fresh state.
func (p *AgentProcess) AwaitInput(req core.Awaitable) core.ActionStatus {
	status := p.signals.parkAwaitable(req)
	if status == core.ActionWaiting {
		p.publishEvent(p.signals.buildWaitingEvent(p.id, req))
	}
	return status
}

// --- event plumbing -------------------------------------------------------

// publishEvent dispatches via the platform's multicast listener and
// the per-process multicast (populated from process-scope EventListener
// extensions). Either may be nil — the function tolerates that.
func (p *AgentProcess) publishEvent(e event.Event) {
	if p.platform != nil {
		p.platform.publish(e)
	}
	if p.processEvents != nil && e != nil {
		p.processEvents.OnEvent(e)
	}
}

// baseEvent stamps a fresh [event.BaseEvent] tagged with this process's
// id. Convenience used by every event the runtime emits — keeps the
// per-event struct literals one liner short.
func (p *AgentProcess) baseEvent() event.BaseEvent {
	return event.NewBaseEvent(p.id)
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

// Tracing attribute keys shared between process- and action-level
// spans. Centralized at the AgentProcess scope (where they originate)
// because external listeners — dashboards, exporters — key off the
// stable string values. Treat as schema; renaming breaks consumers.
const (
	attrAgentName = "lynx.agent.name"
	attrProcessID = "lynx.agent.process_id"
)

// startTickSpan creates a span scoped to one tick.
func (p *AgentProcess) startTickSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return core.AgentTracer().Start(ctx, name,
		trace.WithAttributes(
			attribute.String(attrAgentName, p.agent.Name),
			attribute.String(attrProcessID, p.id),
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

// --- extension accessors -------------------------------------------------
//
// platformExtensions / processExtensions return the two raw lists;
// combinedExtensions / combinedExtensionsResolverFirst pre-merge them in
// the orders the dispatch helpers expect. Kept on AgentProcess (not
// Platform) so process-scope and platform-scope concerns are visible
// from one place.

// platformServices returns the platform's open service registry, or a
// fresh empty one when there's no platform attached (test fixtures).
func (p *AgentProcess) platformServices() *core.ServiceProvider {
	if p.platform == nil {
		return core.NewServiceProvider()
	}
	return p.platform.services
}

// platformChatClient returns the platform's shared [chat.Client], or
// nil when the platform was constructed without one (or when there's
// no platform attached — test fixtures). Action code reaches this via
// ProcessContext.Chat / ChatWithActionTools.
func (p *AgentProcess) platformChatClient() *chat.Client {
	if p.platform == nil {
		return nil
	}
	return p.platform.chatClient
}

// platformGuardrails returns the platform-level chat guardrails, or
// nil when none are configured (or no platform attached). Threaded
// into ProcessContext so [ProcessContext.Chat] can pre-install the
// global middlewares on every request.
func (p *AgentProcess) platformGuardrails() *core.Guardrails {
	if p.platform == nil {
		return nil
	}
	return p.platform.guardrails
}

// platformExtensions exposes the platform-scoped extension list.
func (p *AgentProcess) platformExtensions() []core.Extension {
	if p.platform == nil {
		return nil
	}
	return p.platform.extensions.list
}

// processExtensions exposes the per-process extension list (from
// [core.ProcessOptions.Extensions]).
func (p *AgentProcess) processExtensions() []core.Extension {
	if p.options == nil {
		return nil
	}
	return p.options.Extensions
}

// combinedExtensions returns platform extensions followed by process
// extensions — the natural ordering for onion / wrap chains where
// platform sits outermost (registered earliest) and process sits
// innermost (registered last). Goal-approver dispatch reads this list.
func (p *AgentProcess) combinedExtensions() []core.Extension {
	return mergeExtensions(p.platformExtensions(), p.processExtensions())
}

// combinedExtensionsResolverFirst returns process extensions BEFORE
// platform extensions — the order used for first-hit resolvers so a
// process-scope override is consulted first.
func (p *AgentProcess) combinedExtensionsResolverFirst() []core.Extension {
	return mergeExtensions(p.processExtensions(), p.platformExtensions())
}

// mergeExtensions concatenates first then second, returning the input
// directly (no allocation) when either side is empty.
func mergeExtensions(first, second []core.Extension) []core.Extension {
	if len(second) == 0 {
		return first
	}
	if len(first) == 0 {
		return second
	}
	out := make([]core.Extension, 0, len(first)+len(second))
	out = append(out, first...)
	out = append(out, second...)
	return out
}
