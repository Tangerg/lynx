package agentexec

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/component/toolresultpreview"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
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
	ApproveToolCall(ctx context.Context, callID, toolName, arguments string, target ToolApprovalTarget) ToolApprovalVerdict

	OnToolCallStart(callID, toolName, arguments string)
	OnToolCallEnd(callID, toolName, arguments, output string, ref *offload.Ref, mutatedPaths []string, err error)

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

// ToolApprovalTarget carries the capabilities of the exact tool wrapper being
// gated. MCP identifies the immutable upstream bound to that wrapper, avoiding
// a second lookup through a live catalog that may have changed since the turn
// resolved its tools. Its zero value denotes a non-MCP tool.
type ToolApprovalTarget struct {
	FileMutations tools.FileMutationReporter
	MCP           mcpserver.ToolRef
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

	// evictStore + evictThreshold drive tool-result eviction at the observation
	// chokepoint: an oversized successful result is offloaded to the store and
	// replaced — for BOTH the transcript (via OnToolCallEnd) and the model (the
	// value returned to the tool loop) — by a head+tail preview carrying the
	// model-facing blob id; the observer also emits the typed relationship used
	// by persistence. The full body lives in exactly one place. nil store / a
	// non-positive threshold disables it.
	evictStore     toolResultOffloader
	evictThreshold int

	mu sync.Mutex
	// model is keyed by process ownership as well as the provider-defined call
	// id. Root and child processes commonly reuse ids such as "call_1" while
	// running concurrently; keying by call id alone lets one process overwrite
	// another and can leave the displaced call waiting forever on started.
	model      map[processToolCallKey]*observedModelCall
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

type processToolCallKey struct {
	processID string
	callID    string
}

func newToolObservation(target toolObserver, evictStore toolResultOffloader, evictThreshold int) *toolObservation {
	if toolObserverIsNil(target) {
		return nil
	}
	return &toolObservation{
		target:         target,
		evictStore:     evictStore,
		evictThreshold: evictThreshold,
		model:          make(map[processToolCallKey]*observedModelCall),
		finished:       make(map[string]struct{}),
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
	o.model[processToolCallKey{processID: processID, callID: call.ID}] = observed
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
	key := processToolCallKey{processID: process.ID(), callID: call.ID}
	o.mu.Lock()
	observed, ok := o.model[key]
	o.mu.Unlock()
	if !ok || observed.name != name || observed.arguments != arguments {
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

func (o *toolObservation) finish(call *observedModelCall, bound bool, arguments, output string, ref *offload.Ref, mutatedPaths []string, err error) {
	if bound {
		o.mu.Lock()
		o.finished[call.id] = struct{}{}
		o.mu.Unlock()
	}
	o.target.OnToolCallEnd(call.id, call.name, arguments, output, ref, mutatedPaths, err)
}

// result closes canonical calls that never reached a resolved tool wrapper,
// such as an unknown tool. Wrapped calls have already emitted their richer
// completion (effective arguments, mutation paths, and original error), so the
// matching model result only retires the deduplication marker.
func (o *toolObservation) result(processID string, round int, result chat.ToolResult) {
	id := modelToolCallID(processID, round, result.ID)
	key := processToolCallKey{processID: processID, callID: result.ID}
	o.mu.Lock()
	if _, ok := o.finished[id]; ok {
		delete(o.finished, id)
		if call := o.model[key]; call != nil && call.id == id {
			delete(o.model, key)
		}
		o.mu.Unlock()
		return
	}
	call := o.model[key]
	if call == nil || call.id != id {
		// A restored checkpoint may publish a result for a sibling that
		// completed before the process parked. Its lifecycle was durably
		// committed with that park; no boundary was begun in this observer
		// instance, so emitting it again would duplicate the transcript.
		o.mu.Unlock()
		return
	}
	call.prepared = true
	ready := o.readyStartsLocked()
	delete(o.model, key)
	o.mu.Unlock()
	o.publishStarts(ready)
	<-call.started

	var err error
	if result.IsError {
		err = errors.New(result.Result)
	}
	o.target.OnToolCallEnd(id, result.Name, call.arguments, result.Result, nil, nil, err)
}

// evict offloads an oversized successful tool result to the blob store and
// returns a head+tail preview plus its typed blob reference; it returns output
// unchanged when eviction is disabled, the output fits, there is no session to
// scope the blob under, the tool is the read-back tool (evicting its output
// would loop), or the offload fails (best-effort — degrade to the full body
// rather than fail an otherwise-successful call).
func (o *toolObservation) evict(ctx context.Context, toolName, output string) (string, *offload.Ref) {
	if o.evictStore == nil || o.evictThreshold <= 0 || len(output) <= o.evictThreshold {
		return output, nil
	}
	if toolName == toolport.ToolNameReadToolResult {
		return output, nil
	}
	sessionID := turnctx.TurnSession(ctx)
	if sessionID == "" {
		return output, nil
	}
	id := offload.NewID()
	preview := toolresultpreview.Render(output, string(id), toolport.ToolNameReadToolResult, min(toolResultPreviewBytes, o.evictThreshold))
	if len(preview) >= len(output) {
		// Very small configured thresholds can make the retrieval marker larger
		// than the body. Keep the body inline without staging any durable state.
		return output, nil
	}
	if err := o.evictStore.Stage(ctx, offload.ToolResultStage{
		ID: id, SessionID: sessionID, ToolName: toolName, Body: output,
	}); err != nil {
		return output, nil
	}
	return preview, &offload.Ref{ID: id}
}
