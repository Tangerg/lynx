package tool

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
)

// InvocationResult is what the tool-calling middleware emits after
// running the LLM-requested tool calls. It captures the inline results
// (toolMessage) plus the flow-control bit (allReturnDirect) that decides
// whether to feed results back to the LLM or return them to the caller.
type InvocationResult struct {
	request         *chat.Request
	response        *chat.Response
	toolMessage     *chat.ToolMessage
	allReturnDirect bool

	// interrupt is set when a tool call interrupted the loop pending human
	// input (HITL). It carries the results produced so far this round plus
	// the interrupt cause so the middleware can assemble the resumable
	// conversation and propagate. When set, toolMessage is nil and the
	// normal continue/return path is bypassed.
	interrupt *roundInterrupt
}

// Interrupted reports whether the round interrupted pending human input.
func (r *InvocationResult) Interrupted() bool { return r.interrupt != nil }

// ShouldContinue reports whether the runtime should re-prompt the LLM
// with the tool results. It is true when at least one internal tool
// wants its result fed back to the LLM.
func (r *InvocationResult) ShouldContinue() bool {
	return !r.allReturnDirect
}

// ShouldReturn is the inverse of [InvocationResult.ShouldContinue].
func (r *InvocationResult) ShouldReturn() bool { return !r.ShouldContinue() }

// BuildContinueRequest assembles the next [*chat.Request] in the tool-calling
// loop: the turn's system header, this round's assistant tool-call message,
// and the [*chat.ToolMessage] carrying the inline results. Returns an error when
// the result is not actually in "continue" state.
//
// It does NOT carry the prior conversation — the memory middleware below the
// loop owns the stored history and splices it back in. But it DOES carry the
// assistant tool-call message alongside its tool result: the two form one
// atomic exchange the memory layer persists together. (The memory layer
// deliberately skips persisting a tool-call assistant on its own, so it can't
// strand an unanswered assistant(tool_calls) in the store if the turn
// interrupts mid-round.) Re-sending the FULL conversation, by contrast, is the
// coupling that forced the memory layer to de-duplicate — so only the system
// header and this round's new exchange travel down.
func (r *InvocationResult) BuildContinueRequest() (*chat.Request, error) {
	if !r.ShouldContinue() {
		return nil, errors.New("tool.InvocationResult.BuildContinueRequest: result is in return-direct state")
	}
	if err := r.assertContinuableState(); err != nil {
		return nil, err
	}

	result := r.response.Result
	if result == nil || !result.AssistantMessage.HasToolCalls() {
		return nil, errors.New("tool.InvocationResult.BuildContinueRequest: response has no tool calls")
	}

	msgs := append(systemMessages(r.request.Messages), result.AssistantMessage, r.toolMessage)
	next, err := chat.NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = r.request.Options.Clone()
	next.Tools = slices.Clone(r.request.Tools)
	next.Params = maps.Clone(r.request.Params)
	return next, nil
}

// assertContinuableState validates that the inputs needed to build the
// continuation request are present.
func (r *InvocationResult) assertContinuableState() error {
	if r.request == nil {
		return errors.New("tool.InvocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("tool.InvocationResult: LLM response is missing")
	}
	if r.toolMessage == nil {
		return errors.New("tool.InvocationResult: internal-tools message is missing")
	}
	return nil
}

// BuildReturnResponse assembles the final [*chat.Response] when no further
// LLM round is needed — every internal tool was return-direct.
func (r *InvocationResult) BuildReturnResponse() (*chat.Response, error) {
	if !r.ShouldReturn() {
		return nil, errors.New("tool.InvocationResult.BuildReturnResponse: result is in continue state")
	}
	if r.response == nil {
		return nil, errors.New("tool.InvocationResult.BuildReturnResponse: LLM response is missing")
	}

	withCalls := r.response.Result
	if withCalls == nil || !withCalls.AssistantMessage.HasToolCalls() {
		return nil, errors.New("tool.InvocationResult.BuildReturnResponse: response has no tool calls")
	}

	result, err := chat.NewResult(withCalls.AssistantMessage, withCalls.Metadata)
	if err != nil {
		return nil, fmt.Errorf("tool.InvocationResult.BuildReturnResponse: %w", err)
	}
	result.ToolMessage = r.toolMessage

	return chat.NewResponse(result, r.response.Metadata)
}

// validate ensures the result has the inline tool message populated.
func (r *InvocationResult) validate() error {
	if r.request == nil {
		return errors.New("tool.InvocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("tool.InvocationResult: LLM response is missing")
	}
	if r.toolMessage == nil {
		return errors.New("tool.InvocationResult: internal-tools message is required")
	}
	return nil
}

// callInvoker drives one round of tool invocations: validate every
// requested tool, execute each in order, and assemble the
// [*InvocationResult].
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
type callInvoker struct {
	registry *Registry
}

// newCallInvoker pairs an invoker with its registry.
func newCallInvoker(registry *Registry) *callInvoker {
	return &callInvoker{registry: registry}
}

// canInvokeToolCalls reports whether the response carries tool calls to run.
// Returns (false, nil) when there are none. Unknown tool names are NOT
// rejected here — they are tolerated and turned into error results by
// invokeToolCalls (the model named a tool that doesn't exist; that's
// recoverable feedback, not a reason to abort the run).
func (i *callInvoker) canInvokeToolCalls(resp *chat.Response) (bool, error) {
	if resp.Result == nil || !resp.Result.AssistantMessage.HasToolCalls() {
		return false, nil
	}
	return true, nil
}

// unknownToolResult is the synthetic tool result fed back to the model when
// it calls a tool that isn't registered. It names the missing tool and lists
// the available ones so the model can recover.
func unknownToolResult(name string, available []string) string {
	sorted := slices.Clone(available)
	slices.Sort(sorted)
	if len(sorted) == 0 {
		return fmt.Sprintf("error: tool %q is not available, and no tools are registered", name)
	}
	return fmt.Sprintf("error: tool %q is not available. Available tools: %s", name, strings.Join(sorted, ", "))
}

// toolErrorResult is the synthetic tool result fed back to the model when a
// tool execution fails recoverably, so the model sees the failure and can
// adjust instead of the whole request aborting. The error string is the
// tool's own (already wrapped by the tool); the invoker does not add its
// internal call path.
func toolErrorResult(name string, err error) string {
	return fmt.Sprintf("error: tool %q failed: %s", name, err.Error())
}

// abortsToolLoop reports whether a tool error must PROPAGATE (abort the loop)
// instead of being fed back to the model as a recoverable result. Two cases:
// context cancellation / deadline (the run is being torn down), and a
// [chat.ToolHalt] whose Abort() is true — a fatal failure the model can't fix.
// (A ToolHalt whose Abort() is false is a HITL interrupt, handled separately
// by [interruptsToolLoop] so it carries a resume checkpoint.)
func abortsToolLoop(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	h, ok := errors.AsType[chat.ToolHalt](err)
	return ok && h.Abort()
}

// invokeToolCalls runs every requested tool in order and collects the
// results into a single [*chat.ToolMessage]. One child span per tool call
// is emitted under the parent chat span, tagged with `gen_ai.tool.*`
// attributes — see [toolTracer] / doc/OBSERVABILITY.md §4.5.
func (i *callInvoker) invokeToolCalls(ctx context.Context, calls []*chat.ToolCallPart) (*InvocationResult, error) {
	allReturnDirect := true
	returns := make([]*chat.ToolReturn, 0, len(calls))

	for _, call := range calls {
		t, exists := i.registry.Find(call.Name)
		if !exists {
			// The model named a tool that isn't registered. Answer the call
			// with an error result so it can self-correct, and force a
			// follow-up round — never abort the run over a hallucinated name.
			allReturnDirect = false
			returns = append(returns, &chat.ToolReturn{
				ID:     call.ID,
				Name:   call.Name,
				Result: unknownToolResult(call.Name, i.registry.Names()),
			})
			continue
		}

		content, err := i.invokeOne(ctx, t, call)
		if err != nil {
			if interruptsToolLoop(err) {
				// HITL: this call interrupts the loop pending human input.
				// Stop the round here and report the results produced so far
				// plus the interrupt cause; the middleware assembles the
				// resumable conversation and propagates. Checked before the
				// abort / feedback carve-outs below so interrupt wins.
				return &InvocationResult{
					interrupt: &roundInterrupt{done: returns, cause: err},
				}, nil
			}
			if abortsToolLoop(err) {
				return nil, fmt.Errorf("tool.callInvoker.invokeToolCalls: tool %q failed: %w", call.Name, err)
			}
			// Recoverable failure: feed it back to the model as the tool's
			// result so it can adjust and continue, rather than aborting the
			// run. This is the unconditional default — only control-flow errors
			// (HITL interrupt above, context cancel / ToolHalt-abort here) stop
			// the loop. The failure is also recorded out-of-band on the
			// tool-call item (via the tool observer) — see lyra engine.
			allReturnDirect = false
			returns = append(returns, &chat.ToolReturn{
				ID:     call.ID,
				Name:   call.Name,
				Result: toolErrorResult(call.Name, err),
			})
			continue
		}

		allReturnDirect = allReturnDirect && t.Metadata().ReturnDirect
		returns = append(returns, &chat.ToolReturn{
			ID:     call.ID,
			Name:   call.Name,
			Result: content,
		})
	}

	toolMsg, err := chat.NewToolMessage(returns)
	if err != nil {
		return nil, fmt.Errorf("tool.callInvoker.invokeToolCalls: %w", err)
	}

	return &InvocationResult{
		toolMessage:     toolMsg,
		allReturnDirect: allReturnDirect,
	}, nil
}

// invokeOne dispatches a single tool call under its own OTel span.
// The span emits `gen_ai.tool.name` / `gen_ai.tool.call.id`; an error
// adds the error to the span and sets span status before
// re-throwing the underlying error to the caller. No-op overhead
// when no TracerProvider is configured.
func (i *callInvoker) invokeOne(ctx context.Context, t chat.Tool, call *chat.ToolCallPart) (string, error) {
	ctx, span := toolTracer.Start(ctx, "tool.invoke "+call.Name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(attrToolName, call.Name),
			attribute.String(attrToolCallID, call.ID),
		),
	)
	defer span.End()

	content, err := t.Call(ctx, call.Arguments)

	if err != nil {
		if interruptsToolLoop(err) {
			// HITL interrupt: the tool asked to pause for human input — normal
			// control flow, not a failure. Record it as an event but leave the
			// span status unset (no false error-rate alerts on every approval).
			span.AddEvent("tool_loop.interrupted")
		} else {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}
	return content, err
}

// invoke is the orchestrator: validate, run, attach context.
func (i *callInvoker) invoke(ctx context.Context, req *chat.Request, resp *chat.Response) (*InvocationResult, error) {
	canInvoke, err := i.canInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !canInvoke {
		return nil, errors.New("tool.callInvoker.invoke: response has no valid tool calls")
	}

	result, err := i.invokeToolCalls(ctx, resp.Result.AssistantMessage.CollectToolCalls())
	if err != nil {
		return nil, err
	}
	result.request = req
	result.response = resp

	if result.interrupt != nil {
		// Interrupted round: toolMessage is intentionally nil. Skip validate
		// (which requires it) — the middleware assembles the resumable
		// conversation and propagates the interrupt.
		return result, nil
	}
	return result, result.validate()
}
