package toolloop

import (
	"context"
	"fmt"
	"runtime/debug"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
)

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
