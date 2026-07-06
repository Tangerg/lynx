package toolloop

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
	chatconversation "github.com/Tangerg/lynx/core/model/chat/conversation"
)

// ParkState is the resumable state of an interrupted tool round:
// the assistant message that triggered the tool call(s) and any
// results already produced before the interrupt. On resume the
// conversation tail (assistant + Done) is injected into the request
// so [parseResumePoint] detects it and continues at the pending call.
type ParkState struct {
	Assistant *chat.AssistantMessage `json:"assistant"`
	Done      []*chat.ToolReturn     `json:"done,omitempty"`
}

// ParkConsumer atomically loads AND removes the parked round for a
// conversation, returning (nil, nil) when nothing is parked. Read-and-remove
// is ONE operation by design: a separate clear that failed after a successful
// read would leave a stale round that the next turn — possibly a brand-new
// one on the same conversation — re-injects, wrongly resuming onto a dead
// tail. Atomic consumption makes resume idempotent.
type ParkConsumer interface {
	Consume(ctx context.Context, conversationID string) (*ParkState, error)
}

type ParkWriter interface {
	Write(ctx context.Context, conversationID string, state *ParkState) error
}

// ParkStore is the resumable-round persistence surface: atomically consume a
// parked round on resume ([ParkConsumer]), write one on interrupt
// ([ParkWriter]). Pass it to [Config.ParkStore]; nil means the middleware
// falls back to [buildInterruptResponse] (the conversation-based tail path
// that the engine intercepts).
type ParkStore interface {
	ParkConsumer
	ParkWriter
}

// restorePark atomically consumes any parked round for the request's
// conversation and injects its tail so [parseResumePoint] detects it and
// resumes at the pending call. Consume reads AND removes in one operation, so
// the round can never linger to hijack a later fresh turn on this
// conversation (the bug a read-then-best-effort-clear had when the clear
// failed). A malformed conversation id, or a consume failure, fails the
// request — parked rounds are keyed by the id, so guessing would resume the
// wrong conversation, and resuming onto a half-consumed tail is worse than
// surfacing the error. Returns the request unchanged when no ParkStore is
// configured or nothing is parked.
func (m *middleware) restorePark(ctx context.Context, req *chat.Request) (*chat.Request, error) {
	parkID, err := chatconversation.ID(req)
	if err != nil {
		return nil, err
	}
	if parkID == "" || m.parkStore == nil {
		return req, nil
	}
	state, err := m.parkStore.Consume(ctx, parkID)
	if err != nil {
		return nil, fmt.Errorf("tool: consume parked round: %w", err)
	}
	if state != nil {
		req = injectParkTail(ctx, req, state)
	}
	return req, nil
}

// injectParkTail appends the parked round's conversation tail
// (assistant + Done tool returns) onto the request's messages
// so [parseResumePoint] detects it and resumes at the pending call.
// The engine always adds a user message on every turn — on resume
// the history middleware replays the full history, so the trailing
// user message is stripped and replaced with the tail.
//
// Failures degrade gracefully (the Done returns are dropped / the
// original request is kept — the run proceeds, only re-running work),
// but they mean park state silently evaporated, so each is recorded
// on the ambient span to stay diagnosable.
func injectParkTail(ctx context.Context, req *chat.Request, state *ParkState) *chat.Request {
	span := trace.SpanFromContext(ctx)
	msgs := req.Messages
	// Strip the trailing user message the engine always adds.
	if len(msgs) > 0 {
		if _, ok := msgs[len(msgs)-1].(*chat.UserMessage); ok {
			msgs = msgs[:len(msgs)-1]
		}
	}
	msgs = append(msgs, state.Assistant)
	if len(state.Done) > 0 {
		if tm, err := chat.NewToolMessage(state.Done); err == nil {
			msgs = append(msgs, tm)
		} else {
			span.RecordError(fmt.Errorf("tool: park-tail injection dropped done results: %w", err))
		}
	}
	next, err := chat.NewRequest(msgs)
	if err != nil {
		span.RecordError(fmt.Errorf("tool: park-tail injection kept original request: %w", err))
		return req
	}
	next.Options = req.Options.Clone()
	next.Tools = slices.Clone(req.Tools)
	next.Params = maps.Clone(req.Params)
	return next
}

// interruptOutcome applies the park-vs-tail policy when a tool round
// halts for human input: with a ParkStore the round parks (persisted
// under the request's conversation id) and the returned response is
// nil; without one it returns the [FinishReasonInterrupt] tail
// the caller re-feeds to resume (conversation-tail design — see
// [Config.ParkStore]). The caller pairs the result with the interrupt
// cause per its own delivery protocol — a single return on the call
// path, the two-yield sequence on the stream path ([yieldInterrupt]).
func (m *middleware) interruptOutcome(ctx context.Context, req *chat.Request, assistant *chat.AssistantMessage, done []*chat.ToolReturn) (*chat.Response, error) {
	if m.parkStore != nil {
		m.savePark(ctx, req, assistant, done)
		return nil, nil
	}
	return buildInterruptResponse(assistant, done)
}

// yieldInterrupt delivers an interrupt outcome on the stream path:
// the tail chunk first (when the round didn't park), then the cause —
// skipping the cause when the consumer already walked away.
func (m *middleware) yieldInterrupt(ctx context.Context, req *chat.Request, assistant *chat.AssistantMessage, ri *roundInterrupt, yield func(*chat.Response, error) bool) {
	tail, err := m.interruptOutcome(ctx, req, assistant, ri.done)
	switch {
	case err != nil:
		yield(nil, err)
	case tail == nil:
		yield(nil, ri.cause)
	default:
		if yield(tail, nil) {
			yield(nil, ri.cause)
		}
	}
}

// savePark persists an interrupted round so it can be resumed later.
// No-op when no ParkStore is configured or no park id is on the request.
func (m *middleware) savePark(ctx context.Context, req *chat.Request, assistant *chat.AssistantMessage, done []*chat.ToolReturn) {
	if m.parkStore == nil {
		return
	}
	// A malformed id was already rejected at the handler entry, so an
	// error here degrades to "no park id" (no persistence).
	id, _ := chatconversation.ID(req)
	if id == "" {
		return
	}
	_ = m.parkStore.Write(ctx, id, &ParkState{
		Assistant: assistant,
		Done:      done,
	})
}
