package chat

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
)

// ToolInvocationResult is what the tool-calling middleware emits after
// running the LLM-requested tool calls. It captures the inline results
// (toolMessage) plus the flow-control bit (allReturnDirect) that decides
// whether to feed results back to the LLM or return them to the caller.
type ToolInvocationResult struct {
	request         *Request
	response        *Response
	toolMessage     *ToolMessage
	allReturnDirect bool

	// interrupt is set when a tool call interrupted the loop pending human
	// input (HITL). It carries the results produced so far this round plus
	// the interrupt cause so the middleware can assemble the resumable
	// conversation and propagate. When set, toolMessage is nil and the
	// normal continue/return path is bypassed.
	interrupt *toolRoundInterrupt
}

// Interrupted reports whether the round interrupted pending human input.
func (r *ToolInvocationResult) Interrupted() bool { return r.interrupt != nil }

// ShouldContinue reports whether the runtime should re-prompt the LLM
// with the tool results. It is true when at least one internal tool
// wants its result fed back to the LLM.
func (r *ToolInvocationResult) ShouldContinue() bool {
	return !r.allReturnDirect
}

// ShouldReturn is the inverse of [ToolInvocationResult.ShouldContinue].
func (r *ToolInvocationResult) ShouldReturn() bool { return !r.ShouldContinue() }

// BuildContinueRequest assembles the next [*Request] in the tool-calling
// loop: the turn's system header, this round's assistant tool-call message,
// and the [*ToolMessage] carrying the inline results. Returns an error when
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
func (r *ToolInvocationResult) BuildContinueRequest() (*Request, error) {
	if !r.ShouldContinue() {
		return nil, errors.New("chat.ToolInvocationResult.BuildContinueRequest: result is in return-direct state")
	}
	if err := r.assertContinuableState(); err != nil {
		return nil, err
	}

	result := r.response.Result
	if result == nil || !result.AssistantMessage.HasToolCalls() {
		return nil, errors.New("chat.ToolInvocationResult.BuildContinueRequest: response has no tool calls")
	}

	msgs := append(systemMessages(r.request.Messages), result.AssistantMessage, r.toolMessage)
	next, err := NewRequest(msgs)
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
func (r *ToolInvocationResult) assertContinuableState() error {
	if r.request == nil {
		return errors.New("chat.ToolInvocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("chat.ToolInvocationResult: LLM response is missing")
	}
	if r.toolMessage == nil {
		return errors.New("chat.ToolInvocationResult: internal-tools message is missing")
	}
	return nil
}

// BuildReturnResponse assembles the final [*Response] when no further
// LLM round is needed — every internal tool was return-direct.
func (r *ToolInvocationResult) BuildReturnResponse() (*Response, error) {
	if !r.ShouldReturn() {
		return nil, errors.New("chat.ToolInvocationResult.BuildReturnResponse: result is in continue state")
	}
	if r.response == nil {
		return nil, errors.New("chat.ToolInvocationResult.BuildReturnResponse: LLM response is missing")
	}

	withCalls := r.response.Result
	if withCalls == nil || !withCalls.AssistantMessage.HasToolCalls() {
		return nil, errors.New("chat.ToolInvocationResult.BuildReturnResponse: response has no tool calls")
	}

	result, err := NewResult(withCalls.AssistantMessage, withCalls.Metadata)
	if err != nil {
		return nil, fmt.Errorf("chat.ToolInvocationResult.BuildReturnResponse: %w", err)
	}
	result.ToolMessage = r.toolMessage

	return NewResponse(result, r.response.Metadata)
}

// validate ensures the result has the inline tool message populated.
func (r *ToolInvocationResult) validate() error {
	if r.request == nil {
		return errors.New("chat.ToolInvocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("chat.ToolInvocationResult: LLM response is missing")
	}
	if r.toolMessage == nil {
		return errors.New("chat.ToolInvocationResult: internal-tools message is required")
	}
	return nil
}

// toolCallInvoker drives one round of tool invocations: validate every
// requested tool, execute each in order, and assemble the
// [*ToolInvocationResult].
type toolCallInvoker struct {
	registry *ToolRegistry

	// feedbackOnUnknown, when set, makes a call to an unregistered tool
	// produce an error result fed back to the model (so it can pick a real
	// tool) instead of aborting the whole request.
	feedbackOnUnknown bool

	// feedbackOnError, when set, turns a tool execution failure into an
	// error result fed back to the model (so it can adjust and continue)
	// instead of aborting the whole request. Sibling of feedbackOnUnknown
	// for execution errors.
	feedbackOnError bool
}

// newToolCallInvoker pairs an invoker with its registry.
func newToolCallInvoker(registry *ToolRegistry) *toolCallInvoker {
	return &toolCallInvoker{registry: registry}
}

// canInvokeToolCalls verifies every requested tool name is registered.
// Returns (false, nil) when the response contains no tool calls at all.
// Returns (false, err) when an unknown tool is requested — unless
// feedbackOnUnknown is set, in which case unknown tools are tolerated and
// turned into error results by invokeToolCalls.
func (i *toolCallInvoker) canInvokeToolCalls(resp *Response) (bool, error) {
	if resp.Result == nil || !resp.Result.AssistantMessage.HasToolCalls() {
		return false, nil
	}

	if i.feedbackOnUnknown {
		return true, nil
	}

	for call := range resp.Result.AssistantMessage.ToolCalls() {
		if _, exists := i.registry.Find(call.Name); !exists {
			return false, fmt.Errorf("chat.toolCallInvoker.canInvokeToolCalls: tool %q not registered", call.Name)
		}
	}
	return true, nil
}

// unknownToolResult is the synthetic tool result fed back to the model when
// it calls a tool that isn't registered (feedbackOnUnknown path). It names
// the missing tool and lists the available ones so the model can recover.
func unknownToolResult(name string, available []string) string {
	sorted := slices.Clone(available)
	slices.Sort(sorted)
	if len(sorted) == 0 {
		return fmt.Sprintf("error: tool %q is not available, and no tools are registered", name)
	}
	return fmt.Sprintf("error: tool %q is not available. Available tools: %s", name, strings.Join(sorted, ", "))
}

// toolErrorResult is the synthetic tool result fed back to the model when a
// tool execution fails (feedbackOnError path), so the model sees the failure
// and can adjust instead of the whole request aborting. The error string is
// the tool's own (already wrapped by the tool); the invoker does not add its
// internal call path.
func toolErrorResult(name string, err error) string {
	return fmt.Sprintf("error: tool %q failed: %s", name, err.Error())
}

// abortsToolLoop reports whether a tool error must PROPAGATE (abort the
// loop) even under FeedbackOnToolError, instead of being fed back to the
// model as a recoverable result. Two carve-outs from "feed back": context
// cancellation (the run is being torn down) and control-flow errors that
// implement ToolLoopAbort() bool returning true — HITL suspension
// (agent/hitl.PauseError) rides this so a parked tool reaches the caller
// intact rather than being masked as a failed tool result.
func abortsToolLoop(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ab interface{ ToolLoopAbort() bool }
	return errors.As(err, &ab) && ab.ToolLoopAbort()
}

// invokeToolCalls runs every requested tool in order and collects the
// results into a single [*ToolMessage]. One child span per tool call
// is emitted under the parent chat span, tagged with `lynx.tool.*`
// attributes — see [toolTracer] / doc/OBSERVABILITY.md §4.5.
func (i *toolCallInvoker) invokeToolCalls(ctx context.Context, calls []*ToolCallPart) (*ToolInvocationResult, error) {
	allReturnDirect := true
	returns := make([]*ToolReturn, 0, len(calls))

	for _, call := range calls {
		t, exists := i.registry.Find(call.Name)
		if !exists {
			// Reachable only with feedbackOnUnknown set (otherwise
			// canInvokeToolCalls already aborted). Answer the tool call
			// with an error result so the model can self-correct, and
			// force a follow-up round.
			allReturnDirect = false
			returns = append(returns, &ToolReturn{
				ID:     call.ID,
				Name:   call.Name,
				Result: unknownToolResult(call.Name, i.registry.Names()),
			})
			continue
		}

		result, err := i.invokeOne(ctx, t, call)
		if err != nil {
			if interruptsToolLoop(err) {
				// HITL: this call interrupts the loop pending human input.
				// Stop the round here and report the results produced so far
				// plus the interrupt cause; the middleware assembles the
				// resumable conversation and propagates. Checked before the
				// abort / feedback carve-outs below so interrupt wins.
				return &ToolInvocationResult{
					interrupt: &toolRoundInterrupt{done: returns, cause: err},
				}, nil
			}
			if !i.feedbackOnError || abortsToolLoop(err) {
				return nil, fmt.Errorf("chat.toolCallInvoker.invokeToolCalls: tool %q failed: %w", call.Name, err)
			}
			// Feed the failure back to the model as the tool's result so it
			// can adjust and continue, rather than aborting the run. The
			// failure is already recorded out-of-band on the tool-call item
			// (via the tool observer) — see lyra engine. Control-flow errors
			// (HITL pause, cancellation) take the propagate path above.
			allReturnDirect = false
			returns = append(returns, &ToolReturn{
				ID:     call.ID,
				Name:   call.Name,
				Result: toolErrorResult(call.Name, err),
			})
			continue
		}

		allReturnDirect = allReturnDirect && t.Metadata().ReturnDirect
		returns = append(returns, &ToolReturn{
			ID:       call.ID,
			Name:     call.Name,
			Result:   result.Content,
			Artifact: result.Artifact,
		})
	}

	toolMsg, err := NewToolMessage(returns)
	if err != nil {
		return nil, fmt.Errorf("chat.toolCallInvoker.invokeToolCalls: %w", err)
	}

	return &ToolInvocationResult{
		toolMessage:     toolMsg,
		allReturnDirect: allReturnDirect,
	}, nil
}

// invokeOne dispatches a single tool call under its own OTel span.
// The span emits `lynx.tool.name` / `lynx.tool.call_id`; an error
// adds `lynx.tool.is_error=true` and sets span status before
// re-throwing the underlying error to the caller. No-op overhead
// when no TracerProvider is configured.
func (i *toolCallInvoker) invokeOne(ctx context.Context, t Tool, call *ToolCallPart) (ToolResult, error) {
	ctx, span := toolTracer.Start(ctx, "tool.invoke "+call.Name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(attrLynxToolName, call.Name),
			attribute.String(attrLynxToolCallID, call.ID),
		),
	)
	defer span.End()

	var (
		result ToolResult
		err    error
	)
	if at, ok := t.(ArtifactTool); ok {
		// Artifact-bearing tools return content + an out-of-band value.
		result, err = at.CallArtifact(ctx, call.Arguments)
	} else {
		var content string
		content, err = t.Call(ctx, call.Arguments)
		result = ToolResult{Content: content}
	}

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
	return result, err
}

// invoke is the orchestrator: validate, run, attach context.
func (i *toolCallInvoker) invoke(ctx context.Context, req *Request, resp *Response) (*ToolInvocationResult, error) {
	canInvoke, err := i.canInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !canInvoke {
		return nil, errors.New("chat.toolCallInvoker.invoke: response has no valid tool calls")
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
