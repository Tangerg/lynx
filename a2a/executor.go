package a2a

import (
	"context"
	"iter"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Agent is the lynx-side capability exposed over A2A. It is intentionally
// narrow — text in, streamed text out — so the consumer (lyra / an agent
// runtime) implements it without this package depending on those layers.
// The interface lives here, in the consumer, per the lynx convention: the
// a2a server is what "runs an agent", so it declares the shape it needs.
type Agent interface {
	// Run handles one inbound A2A message, already flattened to text, and
	// yields the reply as a sequence of text chunks. A single-shot agent
	// yields once; a streaming agent yields deltas. A yielded error ends the
	// task as failed and stops iteration.
	Run(ctx context.Context, input string) iter.Seq2[string, error]
}

// executor adapts an [Agent] to the SDK's [a2asrv.AgentExecutor]: it
// translates the inbound message to text, drives the agent, and maps the
// streamed chunks onto the A2A task lifecycle (working → artifact deltas →
// completed, or failed on error).
type executor struct {
	agent Agent
}

var _ a2asrv.AgentExecutor = (*executor)(nil)

// NewExecutor adapts a lynx [Agent] to an [a2asrv.AgentExecutor], the
// dependency [a2asrv.NewHandler] requires. Use it when wiring the server
// onto a custom mux or transport; [NewHTTPHandler] does this for you.
func NewExecutor(agent Agent) (a2asrv.AgentExecutor, error) {
	if agent == nil {
		return nil, ErrNilAgent
	}
	return &executor{agent: agent}, nil
}

// Execute implements [a2asrv.AgentExecutor]. The first event creates the
// task (the SDK requires a Task or Message first), then each agent chunk is
// an artifact delta, closed by a terminal Completed status — or a Failed
// status carrying the error message if the agent errors mid-stream.
func (e *executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[sdka2a.Event, error] {
	return func(yield func(sdka2a.Event, error) bool) {
		// One server span per task execution — the inbound mirror of the
		// client span in [AgentTool.Call]. Opened when the SDK drains the
		// sequence, closed at the terminal event; a mid-stream agent error
		// is recorded before the Failed terminal goes out.
		ctx, span := a2aTracer.Start(ctx, "a2a.agent.serve",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String(attrTaskID, string(execCtx.TaskID)),
				attribute.String(attrContextID, execCtx.ContextID),
			),
		)
		defer span.End()

		input := ""
		if execCtx.Message != nil {
			input = flattenParts(execCtx.Message.Parts)
		}

		// The task must exist before any status/artifact event, then move to
		// Working — the canonical submitted → working → artifacts → completed
		// lifecycle a streaming A2A consumer expects.
		if !yield(sdka2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
			return
		}
		if !yield(sdka2a.NewStatusUpdateEvent(execCtx, sdka2a.TaskStateWorking, nil), nil) {
			return
		}

		for chunk, err := range e.agent.Run(ctx, input) {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				failure := sdka2a.NewMessage(sdka2a.MessageRoleAgent, sdka2a.NewTextPart(err.Error()))
				yield(sdka2a.NewStatusUpdateEvent(execCtx, sdka2a.TaskStateFailed, failure), nil)
				return
			}
			if chunk == "" {
				continue
			}
			if !yield(sdka2a.NewArtifactEvent(execCtx, sdka2a.NewTextPart(chunk)), nil) {
				return
			}
		}

		yield(sdka2a.NewStatusUpdateEvent(execCtx, sdka2a.TaskStateCompleted, nil), nil)
	}
}

// Cancel implements [a2asrv.AgentExecutor]: it marks the task canceled. The
// in-flight Execute is stopped by the SDK via context cancellation; this
// only records the terminal state.
func (e *executor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[sdka2a.Event, error] {
	return func(yield func(sdka2a.Event, error) bool) {
		yield(sdka2a.NewStatusUpdateEvent(execCtx, sdka2a.TaskStateCanceled, nil), nil)
	}
}
