package toolloop

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

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
