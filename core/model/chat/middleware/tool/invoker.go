package tool

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgSafe "github.com/Tangerg/lynx/pkg/safe"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
	pkgSync "github.com/Tangerg/lynx/pkg/sync"
)

// maxConcurrentToolCalls bounds how many concurrency-safe tool calls run at
// once within one round's parallel batch. A round rarely emits more than a
// handful of parallelizable calls; the cap keeps a model that fans out wide
// (many `task` sub-agents, many reads) from stampeding provider rate limits.
const maxConcurrentToolCalls = 8

// invocationResult is what the tool-calling middleware emits after
// running the LLM-requested tool calls. It captures the inline results
// (toolMessage) plus the flow-control bit (allReturnDirect) that decides
// whether to feed results back to the LLM or return them to the caller.
type invocationResult struct {
	request         *chat.Request
	response        *chat.Response
	toolMessage     *chat.ToolMessage
	allReturnDirect bool

	// interrupt is set when a tool call halted the round for human input
	// (HITL). It carries the results produced so far this round plus the
	// interrupt cause; the middleware turns it into a FinishReasonInterrupt
	// response (the resumable tail) and propagates the cause so the caller
	// parks. When set, toolMessage is nil.
	interrupt *roundInterrupt
}

// roundInterrupt is the partial result of a tool round that halted for human
// input: the results already produced this invocation (done, in call order)
// and the interrupt cause (a [chat.ToolHalt]). The still-pending calls are
// derived from the assistant message at resume time, so they are not carried
// here.
type roundInterrupt struct {
	done  []*chat.ToolReturn
	cause error
}

// shouldContinue reports whether the runtime should re-prompt the LLM
// with the tool results. It is true when at least one internal tool
// wants its result fed back to the LLM.
func (r *invocationResult) shouldContinue() bool {
	return !r.allReturnDirect
}

// shouldReturn is the inverse of [invocationResult.shouldContinue].
func (r *invocationResult) shouldReturn() bool { return !r.shouldContinue() }

// buildContinueRequest assembles the next request when the round wants an LLM
// follow-up. It validates the continue state, then defers the message assembly
// to [nextRoundRequest].
func (r *invocationResult) buildContinueRequest() (*chat.Request, error) {
	if !r.shouldContinue() {
		return nil, errors.New("tool.invocationResult.buildContinueRequest: result is in return-direct state")
	}
	if err := r.validate(); err != nil {
		return nil, err
	}

	result := r.response.Result
	if result == nil || !result.AssistantMessage.HasToolCalls() {
		return nil, errors.New("tool.invocationResult.buildContinueRequest: response has no tool calls")
	}
	return nextRoundRequest(r.request, result.AssistantMessage, r.toolMessage)
}

// buildReturnResponse assembles the final [*chat.Response] when no further
// LLM round is needed — every internal tool was return-direct.
func (r *invocationResult) buildReturnResponse() (*chat.Response, error) {
	if !r.shouldReturn() {
		return nil, errors.New("tool.invocationResult.buildReturnResponse: result is in continue state")
	}
	if r.response == nil {
		return nil, errors.New("tool.invocationResult.buildReturnResponse: LLM response is missing")
	}

	withCalls := r.response.Result
	if withCalls == nil || !withCalls.AssistantMessage.HasToolCalls() {
		return nil, errors.New("tool.invocationResult.buildReturnResponse: response has no tool calls")
	}

	result, err := chat.NewResult(withCalls.AssistantMessage, withCalls.Metadata)
	if err != nil {
		return nil, fmt.Errorf("tool.invocationResult.buildReturnResponse: %w", err)
	}
	result.ToolMessage = r.toolMessage

	return chat.NewResponse(result, r.response.Metadata)
}

// validate ensures the result has the inline tool message populated.
func (r *invocationResult) validate() error {
	if r.request == nil {
		return errors.New("tool.invocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("tool.invocationResult: LLM response is missing")
	}
	if r.toolMessage == nil {
		return errors.New("tool.invocationResult: internal-tools message is required")
	}
	return nil
}

// invoker drives the tool-calling loop's per-round work for both the
// call and stream paths: it owns the loop's tool [registry], decides
// return-direct, validates every requested tool, executes each in
// order, and assembles the [*invocationResult].
//
// Error policy (no knobs — this is the framework default): a tool failure is
// recoverable UNLESS it's a control-flow signal. A control-flow error
// ([abortsToolLoop]: context cancel/deadline or a chat.ToolHalt with Abort()==true, and
// [interruptsToolLoop]: a HITL interrupt) propagates and stops the loop;
// EVERYTHING else — file-not-found, wrong credentials, a non-zero exit a tool
// chose to surface as an error, an unregistered tool — is turned into a tool
// result and fed back so the model can adjust. A tool author thus picks the
// outcome at the source: fold a failure into the result string for full
// control over the wording, or just return an ordinary error and let the loop
// wrap it. See [chat.Tool.Call].
type invoker struct {
	registry *registry
}

// newInvoker builds an invoker backed by a fresh registry.
// capacityHint, if positive, preallocates the registry's backing map.
func newInvoker(capacityHint ...int) *invoker {
	return &invoker{registry: newRegistry(capacityHint...)}
}

// register adds tools to the loop's registry, keyed by definition name.
func (i *invoker) register(tools ...chat.Tool) {
	i.registry.register(tools...)
}

// shouldReturnDirect reports whether the conversation should end with
// the most recent tool message (no further LLM round). It is true only
// when:
//   - the last message is a [*chat.ToolMessage], AND
//   - every tool referenced in that message is registered, AND
//   - every such tool has ReturnDirect = true.
func (i *invoker) shouldReturnDirect(msgs []chat.Message) bool {
	last, ok := pkgSlices.Last(msgs)
	if !ok {
		return false
	}
	toolMsg, ok := last.(*chat.ToolMessage)
	if !ok {
		return false
	}

	for _, ret := range toolMsg.ToolReturns {
		t, exists := i.registry.find(ret.Name)
		if !exists {
			return false
		}
		if !t.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// buildReturnDirectResponse assembles a synthetic [*chat.Response] that wraps
// the last [*chat.ToolMessage] as the final answer. Returns an error when
// [invoker.shouldReturnDirect] would return false.
func (i *invoker) buildReturnDirectResponse(msgs []chat.Message) (*chat.Response, error) {
	if !i.shouldReturnDirect(msgs) {
		return nil, errors.New("tool.invoker.buildReturnDirectResponse: conditions for return-direct are not met")
	}
	last, _ := pkgSlices.Last(msgs)

	assistantMsg := chat.NewAssistantMessage(map[string]any{
		"created_by": chat.FinishReasonReturnDirect.String(),
	})
	// shouldReturnDirect already verified the tail is a *chat.ToolMessage.
	return toolRoundResponse(assistantMsg, last.(*chat.ToolMessage), chat.FinishReasonReturnDirect)
}

// canInvokeToolCalls reports whether the response carries tool calls to run.
// Unknown tool names are NOT rejected here — they are tolerated and turned
// into error results by invokeToolCalls (the model named a tool that doesn't
// exist; that's recoverable feedback, not a reason to abort the run).
func (i *invoker) canInvokeToolCalls(resp *chat.Response) bool {
	return resp.Result != nil && resp.Result.AssistantMessage.HasToolCalls()
}

// unknownToolResult is the synthetic tool result the invoker feeds back to the
// model when it calls a tool that isn't registered. It names the missing tool
// and lists the invoker's registered tools so the model can recover.
func (i *invoker) unknownToolResult(name string) string {
	available := i.registry.names()
	slices.Sort(available)
	if len(available) == 0 {
		return fmt.Sprintf("error: tool %q is not available, and no tools are registered", name)
	}
	return fmt.Sprintf("error: tool %q is not available. Available tools: %s", name, strings.Join(available, ", "))
}

// toolErrorResult is the synthetic tool result the invoker feeds back to the
// model when a tool execution fails recoverably, so the model sees the failure
// and can adjust instead of the whole request aborting. The error string is
// the tool's own (already wrapped by the tool); the invoker does not add its
// internal call path.
func (i *invoker) toolErrorResult(name string, err error) string {
	return fmt.Sprintf("error: tool %q failed: %s", name, err.Error())
}

// systemMessages returns the system messages of msgs (zero or one in
// practice). The tool loop forwards them on every downstream request so the
// model always sees the turn's system header first; the memory middleware
// never stores system messages, so they ride along with each round.
func systemMessages(msgs []chat.Message) []chat.Message {
	return chat.FilterMessagesByMessageTypes(msgs, chat.MessageTypeSystem)
}

// nextRoundRequest assembles the next model request from the turn's system
// header plus this round's (assistant tool-call, tool result) exchange,
// carrying the live request's options / tools / params. Shared by the normal
// loop ([invocationResult.buildContinueRequest]) and HITL resume
// ([middleware.resumeCall] / [middleware.resumeStream]).
//
// It deliberately does NOT re-send the prior conversation — the memory
// middleware below the loop owns the stored history and splices it back in. The
// assistant tool-call message DOES travel alongside its tool result so the two
// persist as one atomic exchange (memory skips a lone tool-call assistant, so
// it can never strand an unanswered assistant(tool_calls) if the turn
// interrupts mid-round). Re-sending the full conversation, by contrast, is the
// coupling that forced the memory layer to de-duplicate.
func nextRoundRequest(req *chat.Request, assistant *chat.AssistantMessage, toolMsg *chat.ToolMessage) (*chat.Request, error) {
	msgs := append(systemMessages(req.Messages), assistant, toolMsg)
	next, err := chat.NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = req.Options.Clone()
	next.Tools = slices.Clone(req.Tools)
	next.Params = maps.Clone(req.Params)
	return next, nil
}

// abortsToolLoop reports whether a tool error must PROPAGATE (abort the loop)
// instead of being fed back to the model as a recoverable result. Two cases:
// context cancellation / deadline (the run is being torn down), and a
// [chat.ToolHalt] whose Abort() is true — a fatal failure the model can't fix.
// (A ToolHalt whose Abort() is false is a HITL interrupt — see
// [invoker.interruptsToolLoop] — which also propagates but parks rather
// than fails.) Together with interruptsToolLoop it is the invoker's tool-error
// classification policy; stateless but owned by the invoker that applies it.
func (i *invoker) abortsToolLoop(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	h, ok := errors.AsType[chat.ToolHalt](err)
	return ok && h.Abort()
}

// interruptsToolLoop reports whether a tool error is a human-in-the-loop
// INTERRUPT — a [chat.ToolHalt] whose Abort() is false. The loop stops and
// propagates it unchanged (no feedback to the model) so an outer layer can
// park the run and gather input; on resume the parked tail is fed back and the
// loop continues AT the still-pending call (the model is not re-invoked for
// that round). agent/hitl.InterruptError is the reference implementation; the
// contract is duck-typed so this package never imports agent.
func (i *invoker) interruptsToolLoop(err error) bool {
	h, ok := errors.AsType[chat.ToolHalt](err)
	return ok && !h.Abort()
}

// invokeToolCalls runs a round's tool calls and collects the results into a
// single [*chat.ToolMessage]. Calls run in segments: a maximal run of
// non-conflicting calls executes CONCURRENTLY (bounded by
// [maxConcurrentToolCalls]), while an exclusive call — or one whose resource
// conflicts with a call already in the batch — runs alone. Segments execute in
// the model's call order; results land by index so the tool message preserves
// it. One child span per tool call under the parent chat span carries the
// `gen_ai.tool.*` attributes — see [toolTracer] / doc/OBSERVABILITY.md §4.5.
//
// Concurrency policy ([ConcurrentTool]): a tool is exclusive by default
// (serial, the conservative class) — it runs alone; reads / network /
// sub-agents declare themselves parallel (no conflict); file-mutating tools
// declare a per-call key (conflict only on the same path — distinct files run
// in parallel). HITL-interrupting and aborting tools are exclusive, so a parked
// / aborted call is always alone in its segment and the interrupt's done-set is
// well defined.
func (i *invoker) invokeToolCalls(ctx context.Context, calls []*chat.ToolCallPart) (*invocationResult, error) {
	returns := make([]*chat.ToolReturn, len(calls))
	direct := make([]bool, len(calls))

	for pos := 0; pos < len(calls); {
		end := i.segmentEnd(calls, pos)
		interrupt, abort := i.runSegment(ctx, calls, pos, end, returns, direct)
		if abort != nil {
			return nil, abort
		}
		if interrupt != nil {
			// HITL: a call halted the round. Report the results already
			// produced (any segment's completed calls, in any order — resume
			// matches done ↔ pending by tool-call id, not position) plus the
			// cause; the middleware turns this into a FinishReasonInterrupt
			// response and parks. On resume the loop re-enters at the pending
			// calls — never re-running a done call or re-invoking the model.
			return &invocationResult{
				interrupt: &roundInterrupt{done: filledReturns(returns), cause: interrupt},
			}, nil
		}
		pos = end
	}

	allReturnDirect := true
	for _, d := range direct {
		allReturnDirect = allReturnDirect && d
	}
	toolMsg, err := chat.NewToolMessage(returns)
	if err != nil {
		return nil, fmt.Errorf("tool.invoker.invokeToolCalls: %w", err)
	}
	return &invocationResult{
		toolMessage:     toolMsg,
		allReturnDirect: allReturnDirect,
	}, nil
}

// segmentEnd returns the exclusive upper bound of the segment starting at
// start: the longest run of consecutive calls that may execute together. The
// run stops at the first exclusive call (which runs alone) and at the first
// keyed call whose resource is already claimed by an earlier call in the run
// (it must serialize against it). A single exclusive call yields a one-element
// segment.
func (i *invoker) segmentEnd(calls []*chat.ToolCallPart, start int) int {
	concurrent, key := i.concurrencyOf(calls[start])
	if !concurrent {
		return start + 1
	}
	claimed := map[string]struct{}{}
	if key != "" {
		claimed[key] = struct{}{}
	}
	end := start + 1
	for end < len(calls) {
		concurrent, key = i.concurrencyOf(calls[end])
		if !concurrent {
			break
		}
		if key != "" {
			if _, dup := claimed[key]; dup {
				break
			}
			claimed[key] = struct{}{}
		}
		end++
	}
	return end
}

// concurrencyOf reports whether a call may run concurrently with others and the
// resource key it conflicts on, read from the tool's optional [ConcurrentTool]
// capability. A tool that doesn't implement it — or an unregistered one — is
// exclusive (run alone).
func (i *invoker) concurrencyOf(call *chat.ToolCallPart) (concurrent bool, key string) {
	t, ok := i.registry.find(call.Name)
	if !ok {
		return false, ""
	}
	if c, ok := t.(ConcurrentTool); ok {
		key, concurrent = c.ConcurrencyKey(call.Arguments)
		return concurrent, key
	}
	return false, ""
}

// runSegment executes calls[start:end] — a one-element segment inline, a
// multi-element segment concurrently (bounded by [maxConcurrentToolCalls]) —
// writing each call's result into returns[idx] / direct[idx]. It returns the
// lowest-index HITL interrupt and the lowest-index abort among the segment's
// calls; abort takes precedence (the caller propagates it), an interrupt parks
// the round. Both nil on a clean segment. By policy only an exclusive call
// (its own segment) interrupts or aborts, but a parallel batch tolerates either
// defensively.
func (i *invoker) runSegment(ctx context.Context, calls []*chat.ToolCallPart, start, end int, returns []*chat.ToolReturn, direct []bool) (interrupt, abort error) {
	// run executes one call into its result slot, returning a control-flow
	// signal (HITL interrupt or abort) when the call produced no result. runOne
	// folds a recoverable failure into the result itself, so a non-nil return
	// here is always interrupt-or-abort.
	run := func(ctx context.Context, idx int) error {
		out, err := i.runOne(ctx, calls[idx])
		if err != nil {
			return err
		}
		returns[idx], direct[idx] = out.ret, out.direct
		return nil
	}

	errs := make([]error, end-start) // control-flow signal per call; nil = produced a result
	if len(errs) == 1 {
		errs[0] = run(ctx, start) // exclusive call: inline, no goroutine
	} else {
		// Parallel batch, bounded by maxConcurrentToolCalls. Cancel siblings on
		// the first abort so a torn-down run stops promptly; a HITL interrupt
		// does NOT cancel — the other calls finish and their results join the
		// done-set.
		bctx, cancel := context.WithCancel(ctx)
		defer cancel()
		lim := pkgSync.NewLimiter(maxConcurrentToolCalls)
		var wg sync.WaitGroup
		for idx := start; idx < end; idx++ {
			lim.Acquire()
			wg.Go(func() {
				defer lim.Release()
				if err := run(bctx, idx); err != nil {
					errs[idx-start] = err
					if i.abortsToolLoop(err) {
						cancel()
					}
				}
			})
		}
		wg.Wait()
	}

	// Classify in call order: an abort takes precedence (the run can't
	// continue); otherwise the lowest-index HITL interrupt parks the round.
	for off, err := range errs {
		switch {
		case err == nil:
		case i.abortsToolLoop(err):
			return nil, fmt.Errorf("tool.invoker.invokeToolCalls: tool %q failed: %w", calls[start+off].Name, err)
		case interrupt == nil:
			interrupt = err
		}
	}
	return interrupt, nil
}

// toolOutcome is one completed tool call's result plus whether it is eligible
// for return-direct. A control-flow signal (HITL interrupt / abort) is returned
// as runOne's error, not here.
type toolOutcome struct {
	ret    *chat.ToolReturn
	direct bool
}

// runOne executes one tool call and classifies the result. A nil error means
// ret is set: a normal result, a recoverable-failure result (fed back so the
// model adapts), or the unknown-tool result. A non-nil error is a control-flow
// signal the caller classifies — a HITL interrupt ([invoker.interruptsToolLoop])
// or an abort ([invoker.abortsToolLoop], context cancel / ToolHalt-abort).
func (i *invoker) runOne(ctx context.Context, call *chat.ToolCallPart) (toolOutcome, error) {
	t, exists := i.registry.find(call.Name)
	if !exists {
		// The model named a tool that isn't registered. Answer with an error
		// result so it can self-correct — never abort over a hallucinated name.
		return toolOutcome{ret: &chat.ToolReturn{ID: call.ID, Name: call.Name, Result: i.unknownToolResult(call.Name)}}, nil
	}
	content, err := i.invokeOne(ctx, t, call)
	if err != nil {
		if i.interruptsToolLoop(err) || i.abortsToolLoop(err) {
			return toolOutcome{}, err // control flow: caller decides park vs propagate
		}
		// Recoverable failure: fold it into the result so the model can adjust.
		// Also recorded out-of-band on the tool-call item (the tool observer).
		return toolOutcome{ret: &chat.ToolReturn{ID: call.ID, Name: call.Name, Result: i.toolErrorResult(call.Name, err)}}, nil
	}
	return toolOutcome{ret: &chat.ToolReturn{ID: call.ID, Name: call.Name, Result: content}, direct: t.Metadata().ReturnDirect}, nil
}

// filledReturns drops the nil holes a parked round leaves in the indexed
// results slice (the pending calls that didn't complete), yielding the
// done-set in call order.
func filledReturns(returns []*chat.ToolReturn) []*chat.ToolReturn {
	out := make([]*chat.ToolReturn, 0, len(returns))
	for _, r := range returns {
		if r != nil {
			out = append(out, r)
		}
	}
	return out
}

// invokeOne dispatches a single tool call under its own OTel span. The span
// emits `gen_ai.tool.name` / `gen_ai.tool.call.id`; an error (or a recovered
// panic) is recorded on the span and its status set before the error is handed
// back to the caller. No-op overhead when no TracerProvider is configured.
//
// A tool runs arbitrary code, and in a parallel batch it runs in a goroutine
// this package spawns — an escaping panic there has no ancestor recover on its
// stack and would crash the whole process. So the panic is contained HERE, at
// the tool boundary: the full stack lands on the span, and the loop receives a
// concise error. A panic is neither a [chat.ToolHalt] nor a context error, so
// it flows back as an ordinary recoverable failure (folded into the tool result
// and fed to the model) — the loop's default for any non-control-flow error.
func (i *invoker) invokeOne(ctx context.Context, t chat.Tool, call *chat.ToolCallPart) (content string, err error) {
	ctx, span := toolTracer.Start(ctx, "tool.invoke "+call.Name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(attrToolName, call.Name),
			attribute.String(attrToolCallID, call.ID),
		),
	)
	defer span.End()

	// Runs before span.End (defers are LIFO) so the outcome lands on the span.
	defer func() {
		if r := recover(); r != nil {
			span.RecordError(pkgSafe.NewPanicError(r, debug.Stack()))
			span.SetStatus(codes.Error, "tool panicked")
			err = fmt.Errorf("panic: %v", r)
			return
		}
		switch {
		case err == nil:
		case i.interruptsToolLoop(err):
			// HITL interrupt: the tool asked to pause for human input — normal
			// control flow, not a failure. Record it as an event but leave the
			// span status unset (no false error-rate alerts on every approval).
			span.AddEvent("tool_loop.interrupted")
		default:
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	return t.Call(ctx, call.Arguments)
}

// invoke is the orchestrator: validate, run, attach context.
func (i *invoker) invoke(ctx context.Context, req *chat.Request, resp *chat.Response) (*invocationResult, error) {
	if !i.canInvokeToolCalls(resp) {
		return nil, errors.New("tool.invoker.invoke: response has no valid tool calls")
	}

	result, err := i.invokeToolCalls(ctx, resp.Result.AssistantMessage.CollectToolCalls())
	if err != nil {
		return nil, err
	}
	result.request = req
	result.response = resp

	if result.interrupt != nil {
		// Interrupted round: toolMessage is intentionally nil. Skip validate
		// (which requires it) — the middleware builds the FinishReasonInterrupt
		// response and propagates the cause.
		return result, nil
	}
	return result, result.validate()
}
