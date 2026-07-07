package toolloop

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
)

// maxConcurrentToolCalls bounds how many concurrency-safe tool calls run at
// once within one round's parallel batch. A round rarely emits more than a
// handful of parallelizable calls; the cap keeps a model that fans out wide
// (many `task` sub-agents, many reads) from stampeding provider rate limits.
const maxConcurrentToolCalls = 8

type limiter struct {
	slots chan struct{}
}

func newLimiter(cap int) *limiter {
	if cap <= 0 {
		panic("toolloop: concurrent tool call limit must be > 0")
	}
	return &limiter{slots: make(chan struct{}, cap)}
}

func (l *limiter) Acquire() {
	l.slots <- struct{}{}
}

func (l *limiter) Release() {
	<-l.slots
}

// invoker drives the tool-calling loop's per-round work for both the
// call and stream paths: it owns the loop's tool [registry], decides
// return-direct, validates every requested tool, executes each in
// order, and assembles the [*invocationResult].
//
// Error policy (no knobs — this is the framework default): a tool failure is
// recoverable UNLESS it's a control-flow signal. A control-flow error
// ([abortsToolLoop]: context cancel/deadline or a Halt with Abort()==true, and
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

func newInvoker(capacityHint ...int) *invoker {
	return &invoker{registry: newRegistry(capacityHint...)}
}

func (i *invoker) register(tools ...chat.Tool) {
	i.registry.register(tools...)
}

// canInvokeToolCalls reports whether the response carries tool calls to run.
// Unknown tool names are NOT rejected here — they are tolerated and turned
// into error results by invokeToolCalls (the model named a tool that doesn't
// exist; that's recoverable feedback, not a reason to abort the run).
func (i *invoker) canInvokeToolCalls(resp *chat.Response) bool {
	return resp.Result != nil && resp.Result.AssistantMessage.HasToolCalls()
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
		return nil, fmt.Errorf("toolloop.invoker.invokeToolCalls: %w", err)
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
		lim := newLimiter(maxConcurrentToolCalls)
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
			return nil, fmt.Errorf("toolloop.invoker.invokeToolCalls: tool %q failed: %w", calls[start+off].Name, err)
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
// or an abort ([invoker.abortsToolLoop], context cancel / Halt-abort).
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
	return toolOutcome{ret: &chat.ToolReturn{ID: call.ID, Name: call.Name, Result: content}, direct: returnsDirect(t)}, nil
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
// concise error. A panic is neither a [Halt] nor a context error, so
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
			span.RecordError(fmt.Errorf("toolloop.invoker.invokeOne: panic: %v\nstack:\n%s", r, debug.Stack()))
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

func (i *invoker) invoke(ctx context.Context, req *chat.Request, resp *chat.Response) (*invocationResult, error) {
	if !i.canInvokeToolCalls(resp) {
		return nil, errors.New("toolloop.invoker.invoke: response has no valid tool calls")
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
