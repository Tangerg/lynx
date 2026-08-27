package a2a

import (
	"context"
	"iter"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Agent is the lynx-side capability exposed over A2A. It is intentionally
// narrow — text in, streamed text out — so the consumer (an agent
// runtime) implements it without this package depending on those layers.
// The interface lives here, in the consumer, per the convention: the
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

// textArtifact owns the identity of one streamed text result. The first chunk
// creates the artifact; later chunks update that same artifact so the SDK can
// assemble one logical result instead of storing every delta separately.
type textArtifact struct {
	id sdka2a.ArtifactID
}

func (t *textArtifact) append(info sdka2a.TaskInfoProvider, chunk string) *sdka2a.TaskArtifactUpdateEvent {
	part := sdka2a.NewTextPart(chunk)
	if t.id != "" {
		return sdka2a.NewArtifactUpdateEvent(info, t.id, part)
	}
	event := sdka2a.NewArtifactEvent(info, part)
	t.id = event.Artifact.ID
	return event
}

func newExecutor(agent Agent) (*executor, error) {
	if lo.IsNil(agent) {
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
		projection := textProjection{}
		// One server span per task execution. Opened when the SDK drains the
		// sequence, closed at the terminal event; a mid-stream agent error is
		// recorded before the Failed terminal goes out.
		spanCtx, span := a2aTracer.Start(ctx, "a2a.agent.serve",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String(attrTaskID, string(execCtx.TaskID)),
				attribute.String(attrContextID, execCtx.ContextID),
			),
		)
		defer span.End()

		input := ""
		if execCtx.Message != nil {
			input = projection.parts(execCtx.Message.Parts)
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

		fail := func(err error) {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			message := sdka2a.NewMessage(sdka2a.MessageRoleAgent, sdka2a.NewTextPart(err.Error()))
			yield(sdka2a.NewStatusUpdateEvent(execCtx, sdka2a.TaskStateFailed, message), nil)
		}
		sequence := e.agent.Run(spanCtx, input)
		if sequence == nil {
			fail(errNilAgentSequence)
			return
		}
		var artifact textArtifact
		for chunk, err := range sequence {
			if err != nil {
				fail(err)
				return
			}
			if chunk == "" {
				continue
			}
			if !yield(artifact.append(execCtx, chunk), nil) {
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
