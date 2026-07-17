package agentexec

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// ErrToolDenied is the sentinel the gate hands the observer's OnToolCallEnd
// when a tool call is denied by the approval verdict (vs. failing during
// execution). Lets the wire layer render a "denied" terminal distinct from a
// generic tool failure (and from a green success). errors.Is-matchable.
var ErrToolDenied = errors.New("engine.ErrToolDenied: tool call denied by user")

// toolObserver receives both tool-call lifecycle notifications and
// streaming assistant text deltas as a turn unfolds. Each execution attempt
// fires OnToolCallStart; a completed attempt then fires OnToolCallEnd with the
// same opaque CallID. A resumed HITL attempt reuses that ID and starts again.
// Assistant text arrives in zero or more OnMessageDelta calls between (and
// around) tool calls.
//
// Implementations must be safe for concurrent calls: one tool round may
// overlap explicitly safe calls, and separate turns may share one observer
// backend.
type toolObserver interface {
	// ApproveToolCall is the gate consulted BEFORE every tool call.
	// It returns a verdict telling the decorator whether the call runs,
	// is denied (short-circuited to a recoverable result), or must pause
	// the process for user approval (HITL R model, API.md §6): a non-nil
	// Verdict.Interrupt makes the call return the framework's durable
	// Suspension error, parking the process at [core.StatusWaiting]. The
	// client answers via a continuation run.
	//
	// The decider MUST be non-blocking — it records pending / decided
	// state out of band (typically the process blackboard, keyed by the
	// stable tool name + arguments so the verdict matches the same parked
	// tool call when it is re-presented on resume) rather than waiting on a
	// channel.
	//
	// Receives the same callID it will later get on Start / End so the
	// implementation can pair the gate with the lifecycle.
	ApproveToolCall(ctx context.Context, callID, toolName, arguments string) ToolApprovalVerdict

	OnToolCallStart(callID, toolName, arguments string)
	OnToolCallEnd(callID, toolName, arguments, output string, mutatedPaths []string, err error)

	// OnMessageDelta is invoked for every non-empty text chunk the
	// model streams out. Implementations typically append the chunk
	// to a UI buffer or forward it to an event channel.
	OnMessageDelta(text string)

	// OnReasoningDelta is invoked for every non-empty reasoning
	// (extended thinking) chunk the model streams out — distinct
	// from final-text chunks so UIs can render thinking separately
	// (e.g. dimmed, collapsed, or behind a "show reasoning" toggle).
	OnReasoningDelta(text string)

	// OnUsage is invoked once per completed LLM round (right after the
	// round's tokens are recorded into the process budget), carrying the
	// turn's cumulative token roll-up and cost so far. This is the mid-run
	// usage signal — a live "tokens / cost spent" readout — distinct from the
	// final per-turn total that lands on TurnEnd. costUSD is zero when no
	// pricing hook is configured (the wire layer omits it rather than showing
	// a fabricated $0).
	//
	// contextTokens is THIS round's prompt-token count (not cumulative) — the
	// size of the context the model was just sent, i.e. how full the window is
	// right now. It grows across rounds/turns as history accumulates and drops
	// after a compaction, so the client can render a live context-occupancy
	// gauge (distinct from the summed usage, which only ever grows).
	OnUsage(usage accounting.TokenUsage, costUSD float64, contextTokens int64)
}

var toolObservationKey = core.MustDependencyKey[*toolObservation]("lyra.tool_observation")

// ToolApprovalVerdict is the decorator's instruction for one gated tool
// call (API.md §6 HITL). Exactly one outcome applies:
//
//   - Interrupt != nil → suspend the run for human input (R model); the
//     chat tool loop propagates the durable Suspension so the action parks. On resume
//     the gate is consulted again and returns one of the outcomes below.
//   - Denied           → short-circuit with DenyReason as a recoverable
//     tool result (the model adapts), no execution.
//   - zero value       → run the tool. Arguments, when non-empty, overrides
//     the call's arguments (the "approve with edits" affordance).
type ToolApprovalVerdict struct {
	Interrupt  error
	Denied     bool
	DenyReason string
	Arguments  string
}

// observationFrom extracts the coordinator the engine attached to the typed
// process dependency scope via [Engine.StartTurn]. Returns nil when no observer is
// registered — Action bodies treat that as "no streaming hook
// wired" and skip the per-chunk callback.
//
// Lives here because the key and dependency type are Lyra-specific.
func observationFrom(dependencies *core.Dependencies) *toolObservation {
	if dependencies == nil {
		return nil
	}
	observation, _ := core.LookupDependency(dependencies, toolObservationKey)
	return observation
}

// toolObservation joins two different facts without leaking either concern
// across the adapter boundary:
//
//   - the managed interaction publishes model calls in canonical order;
//   - the tool middleware observes their actual, potentially concurrent,
//     execution and effective arguments.
//
// A process owns one coordinator. Model call IDs therefore need only index the
// currently active round locally, while the emitted ID includes process and
// round ownership so root and child agents cannot collide.
type toolObservation struct {
	target toolObserver

	mu         sync.Mutex
	model      map[string]*observedModelCall
	pending    []*observedModelCall
	next       int
	publishing bool
	finished   map[string]struct{}
}

type observedModelCall struct {
	id        string
	processID string
	name      string
	arguments string
	prepared  bool
	started   chan struct{}
}

func newToolObservation(target toolObserver) *toolObservation {
	if toolObserverIsNil(target) {
		return nil
	}
	return &toolObservation{
		target:   target,
		model:    make(map[string]*observedModelCall),
		finished: make(map[string]struct{}),
	}
}

func toolObserverIsNil(target toolObserver) bool {
	if target == nil {
		return true
	}
	value := reflect.ValueOf(target)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func modelToolCallID(processID string, round int, callID string) string {
	// The length prefix makes the encoding injective without constraining
	// provider-defined call IDs or requiring consumers to parse it.
	return fmt.Sprintf("model:%d:%s:%d:%s", len(processID), processID, round, callID)
}

func (o *toolObservation) begin(processID string, round int, call chat.ToolCall) {
	observed := &observedModelCall{
		id: modelToolCallID(processID, round, call.ID), processID: processID,
		name: call.Name, arguments: call.Arguments, started: make(chan struct{}),
	}
	o.mu.Lock()
	o.model[call.ID] = observed
	o.pending = append(o.pending, observed)
	o.mu.Unlock()
}

func (o *toolObservation) invocation(ctx context.Context, name, arguments string) (*observedModelCall, bool) {
	call, ok := agent.ToolCallFromContext(ctx)
	if !ok || call.Name != name || call.Arguments != arguments {
		return nil, false
	}
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return nil, false
	}
	o.mu.Lock()
	observed, ok := o.model[call.ID]
	o.mu.Unlock()
	if !ok || observed.processID != process.ID() || observed.name != name || observed.arguments != arguments {
		return nil, false
	}
	return observed, true
}

// prepare publishes starts in model order, even when concurrent approval gates
// finish out of order. Waiting for the matching callback also guarantees no
// execution completion can overtake its own start.
func (o *toolObservation) prepare(call *observedModelCall, arguments string) {
	o.mu.Lock()
	call.arguments = arguments
	call.prepared = true
	ready := o.readyStartsLocked()
	o.mu.Unlock()
	o.publishStarts(ready)
	<-call.started
}

func (o *toolObservation) readyStartsLocked() []*observedModelCall {
	if o.publishing {
		return nil
	}
	start := o.next
	for o.next < len(o.pending) && o.pending[o.next].prepared {
		o.next++
	}
	ready := append([]*observedModelCall(nil), o.pending[start:o.next]...)
	o.publishing = len(ready) > 0
	return ready
}

func (o *toolObservation) publishStarts(calls []*observedModelCall) {
	for len(calls) > 0 {
		for _, call := range calls {
			o.target.OnToolCallStart(call.id, call.name, call.arguments)
			close(call.started)
		}
		o.mu.Lock()
		o.publishing = false
		calls = o.readyStartsLocked()
		o.mu.Unlock()
	}
}

func (o *toolObservation) finish(call *observedModelCall, bound bool, arguments, output string, mutatedPaths []string, err error) {
	if bound {
		o.mu.Lock()
		o.finished[call.id] = struct{}{}
		o.mu.Unlock()
	}
	o.target.OnToolCallEnd(call.id, call.name, arguments, output, mutatedPaths, err)
}

// result closes canonical calls that never reached a resolved tool wrapper,
// such as an unknown tool. Wrapped calls have already emitted their richer
// completion (effective arguments, mutation paths, and original error), so the
// matching model result only retires the deduplication marker.
func (o *toolObservation) result(processID string, round int, result chat.ToolResult) {
	id := modelToolCallID(processID, round, result.ID)
	o.mu.Lock()
	if _, ok := o.finished[id]; ok {
		delete(o.finished, id)
		delete(o.model, result.ID)
		o.mu.Unlock()
		return
	}
	call := o.model[result.ID]
	if call == nil {
		// A restored checkpoint may publish a result for a sibling that
		// completed before the process parked. Its lifecycle was durably
		// committed with that park; no boundary was begun in this observer
		// instance, so emitting it again would duplicate the transcript.
		o.mu.Unlock()
		return
	}
	call.prepared = true
	ready := o.readyStartsLocked()
	delete(o.model, result.ID)
	o.mu.Unlock()
	o.publishStarts(ready)
	<-call.started

	var err error
	if result.IsError {
		err = errors.New(result.Result)
	}
	o.target.OnToolCallEnd(id, result.Name, call.arguments, result.Result, nil, err)
}

// toolObserverMiddleware is the process-scope [core.ToolMiddleware]
// the engine attaches when [StartTurn] is called with an observer.
// It wraps every resolved [core.AgentTool] with [observedTool] so
// invocations land on the observer without changing the underlying
// tool implementation.
type toolObserverMiddleware struct {
	observation *toolObservation
}

// Name implements [core.Extension]. The constant string is fine —
// process-scope extensions allow name collisions with engine
// scope, and this decorator is process-scoped.
func (d *toolObserverMiddleware) Name() string { return "tool-observer" }

// WrapTool wraps tool with [observedTool], threading the
// observer into every Call so start / end notifications fire.
// Action is intentionally ignored — Lyra emits per-tool, not
// per-action, events.
func (d *toolObserverMiddleware) WrapTool(_ core.ProcessView, _ core.Action, tool tools.Tool) tools.Tool {
	return &observedTool{inner: tool, observation: d.observation}
}

// observedTool is the per-call wrapper. Managed calls reuse the coordinator's
// process+round+model identity; calls made outside ToolLoop receive a fresh ID.
type observedTool struct {
	inner       tools.Tool
	observation *toolObservation
}

func (o *observedTool) Definition() chat.ToolDefinition { return o.inner.Definition() }

// ReturnsDirect forwards the wrapped tool's return-direct declaration. This
// decorator wraps every resolved tool, so dropping the marker would turn
// return-direct tools into regular continuation tools.
func (o *observedTool) ReturnsDirect() bool {
	if direct, ok := o.inner.(interface{ ReturnsDirect() bool }); ok {
		return direct.ReturnsDirect()
	}
	return false
}

// ConcurrencyKey preserves the wrapped tool's optional scheduling contract.
// Observation is per-call and concurrency-safe; wrapping a read-only or
// resource-keyed tool must not silently downgrade it to exclusive execution.
func (o *observedTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if capability, ok := o.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	}); ok {
		return capability.ConcurrencyKey(arguments)
	}
	return "", false
}

func (o *observedTool) Call(ctx context.Context, arguments string) (string, error) {
	name := o.inner.Definition().Name
	call, bound := o.observation.invocation(ctx, name, arguments)
	if !bound {
		call = &observedModelCall{id: "direct:" + rand.Text(), name: name, arguments: arguments}
	}

	v := o.observation.target.ApproveToolCall(ctx, call.id, name, arguments)
	if v.Arguments != "" {
		arguments = v.Arguments
	}
	if bound {
		o.observation.prepare(call, arguments)
	} else {
		o.observation.target.OnToolCallStart(call.id, name, arguments)
	}
	switch {
	case v.Interrupt != nil:
		return "", v.Interrupt
	case v.Denied:
		// Recoverable denial: the model sees DenyReason as the tool
		// result and adapts instead of aborting. Start/End still fire so
		// UI counts stay matched; End carries ErrToolDenied so the wire
		// renders a distinct "denied" terminal (not a green success).
		o.observation.finish(call, bound, arguments, v.DenyReason, nil, ErrToolDenied)
		return v.DenyReason, nil
	}

	output, err := o.inner.Call(ctx, arguments)
	o.observation.finish(call, bound, arguments, output, o.successfulMutationPaths(arguments, err), err)

	return output, err
}

func (o *observedTool) successfulMutationPaths(arguments string, callErr error) []string {
	if callErr != nil {
		return nil
	}
	reporter, ok := o.inner.(tools.FileMutationReporter)
	if !ok {
		return nil
	}
	paths, err := reporter.MutationPaths(arguments)
	if err != nil {
		return nil
	}
	paths = slices.Clone(paths)
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}
