package memory

import (
	"context"
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

const (
	// ConversationIDKey is the [chat.Request].Params key that identifies
	// the conversation. Set it before calling the model:
	//
	//	req.Set(memory.ConversationIDKey, "session-42")
	//
	// When the key is absent the middleware short-circuits — no history
	// load, no save.
	ConversationIDKey = "lynx:ai:model:chat:memory:conversation_id"

	// SavedMarkerKey is the metadata key the middleware writes onto each
	// message it has persisted. The presence of the marker prevents a
	// duplicate save when the caller passes history back in on a
	// subsequent turn.
	SavedMarkerKey = "lynx:ai:model:chat:memory:saved_marker"
)

// savedMarker is a zero-size sentinel used as the metadata value under
// [SavedMarkerKey]. Using an empty struct keeps memory overhead at zero.
type savedMarker struct{}

// resumedTurnCtxKey marks a chat call as a continuation segment of a turn
// suspended mid-flight (HITL resume).
type resumedTurnCtxKey struct{}

// WithResumedTurn marks ctx as a continuation segment of a suspended turn.
// The middleware then SKIPS its input-side work — loading history and
// persisting the request messages — because the first segment already did
// both, and the tool loop resumes from its own checkpoint (ignoring these
// request messages). The final result is still saved. Without this, every
// resume re-persists the system/user messages, duplicating them in the
// stored conversation. Set by the agent layer when it resumes a turn whose
// tool loop has a saved checkpoint.
func WithResumedTurn(ctx context.Context) context.Context {
	return context.WithValue(ctx, resumedTurnCtxKey{}, struct{}{})
}

// isResumedTurn reports whether ctx was marked by [WithResumedTurn].
func isResumedTurn(ctx context.Context) bool {
	return ctx != nil && ctx.Value(resumedTurnCtxKey{}) != nil
}

// middleware loads conversation history before each request
// and saves new messages afterwards. It deduplicates via [SavedMarkerKey]
// metadata so callers who pass history explicitly never trigger
// duplicate writes.
type middleware struct {
	store Store
}

// NewMiddleware constructs a memory-management middleware backed by
// store. Returns the call/stream middleware pair plus an error when
// store is nil.
//
// Example:
//
//	store := memory.NewInMemoryStore()
//	callMW, streamMW, err := memory.NewMiddleware(store)
//	if err != nil { return err }
//	resp, err := client.Chat().
//	    WithParams(map[string]any{memory.ConversationIDKey: "user-1"}).
//	    WithMiddlewares(callMW, streamMW).
//	    WithUserPrompt("hi").
//	    Call().Response(ctx)
func NewMiddleware(store Store) (chat.CallMiddleware, chat.StreamMiddleware, error) {
	if store == nil {
		return nil, nil, errors.New("memory.NewMiddleware: store must not be nil")
	}
	mw := &middleware{store: store}
	return mw.wrapCallHandler, mw.wrapStreamHandler, nil
}

// extractConversationID returns the conversation id stashed under
// [ConversationIDKey], or "" when the caller did not supply one.
// Returns an error if the value exists but is the wrong type.
func (m *middleware) extractConversationID(req *chat.Request) (string, error) {
	raw, exists := req.Get(ConversationIDKey)
	if !exists {
		return "", nil
	}

	id, ok := raw.(string)
	if !ok {
		return "", errors.New("memory: ConversationIDKey value must be a string")
	}
	return id, nil
}

// isMessageSaved reports whether msg already carries [SavedMarkerKey].
func (m *middleware) isMessageSaved(msg chat.Message) bool {
	meta := msg.Meta()
	if meta == nil {
		return false
	}
	_, ok := meta[SavedMarkerKey].(savedMarker)
	return ok
}

// markMessageAsSaved annotates msg with [SavedMarkerKey] so subsequent
// turns recognize it as already-persisted history.
func (m *middleware) markMessageAsSaved(msg chat.Message) {
	meta := msg.Meta()
	if meta == nil {
		return
	}
	meta[SavedMarkerKey] = savedMarker{}
}

// filterUnsavedMessages returns only those messages that have not yet
// been persisted.
func (m *middleware) filterUnsavedMessages(msgs []chat.Message) []chat.Message {
	out := make([]chat.Message, 0, len(msgs))
	for _, msg := range msgs {
		if !m.isMessageSaved(msg) {
			out = append(out, msg)
		}
	}
	return out
}

// retrieveHistoryMessages loads stored history for the conversation
// referenced by req. Returned messages are pre-marked saved so they are
// not re-persisted by [middleware.persistMessages]. Returns
// nil history when the request carries no conversation id.
func (m *middleware) retrieveHistoryMessages(ctx context.Context, req *chat.Request) ([]chat.Message, error) {
	id, err := m.extractConversationID(req)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	history, err := m.store.Read(ctx, id)
	if err != nil {
		return nil, err
	}

	for _, msg := range history {
		m.markMessageAsSaved(msg)
	}
	return history, nil
}

// persistMessages writes msgs under the request's conversation id and
// marks them saved on success. No-op when no id is set or msgs is empty.
func (m *middleware) persistMessages(ctx context.Context, req *chat.Request, msgs ...chat.Message) error {
	id, err := m.extractConversationID(req)
	if err != nil {
		return err
	}
	if id == "" || len(msgs) == 0 {
		return nil
	}

	if err := m.store.Write(ctx, id, msgs...); err != nil {
		return err
	}

	for _, msg := range msgs {
		m.markMessageAsSaved(msg)
	}
	return nil
}

// prepareRequest is the pre-call step:
//  1. load history,
//  2. persist any new (unsaved) messages from req,
//  3. assemble a fresh [*chat.Request] containing history + new messages.
//
// Options and Params from the original request are cloned onto the new
// one so the underlying handler sees an equivalent request shape.
func (m *middleware) prepareRequest(ctx context.Context, req *chat.Request) (*chat.Request, error) {
	history, err := m.retrieveHistoryMessages(ctx, req)
	if err != nil {
		return nil, err
	}

	newMsgs := m.filterUnsavedMessages(req.Messages)
	if len(newMsgs) > 0 {
		if err := m.persistMessages(ctx, req, newMsgs...); err != nil {
			return nil, err
		}
	}

	combined := append(history, newMsgs...)
	next, err := chat.NewRequest(combined)
	if err != nil {
		return nil, err
	}
	next.Options = req.Options.Clone()
	next.Params = maps.Clone(req.Params)
	next.Tools = slices.Clone(req.Tools)

	return next, nil
}

// saveResponseMessages persists the assistant + tool messages produced
// by the model. AI-generated messages are always new, so no dedup is
// needed.
func (m *middleware) saveResponseMessages(ctx context.Context, req *chat.Request, resp *chat.Response) error {
	if resp.Result == nil {
		return nil
	}
	msgs := []chat.Message{resp.Result.AssistantMessage}
	if resp.Result.ToolMessage != nil {
		msgs = append(msgs, resp.Result.ToolMessage)
	}
	return m.persistMessages(ctx, req, msgs...)
}

// blankAssistant reports whether an assistant message carries neither tool
// calls nor non-whitespace text — a round boundary with nothing to persist.
func blankAssistant(am *chat.AssistantMessage) bool {
	return am == nil || (!am.HasToolCalls() && strings.TrimSpace(am.JoinedText()) == "")
}

// executeCall is the synchronous flow: prepare → call → save.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	if isResumedTurn(ctx) {
		// Continuation of a suspended turn: input was persisted on the first
		// segment, so pass the request straight through (no load) and save
		// the final reply. The synchronous Call path exposes only the final
		// response (no per-round tool messages), so it persists the answer
		// without the round trace — valid, just lossy. lyra drives HITL via
		// the streaming path (executeStream), which persists the full ordered
		// sequence; this branch is a non-lyra fallback.
		resp, err := next.Call(ctx, req)
		if err != nil {
			return nil, err
		}
		if err := m.saveResponseMessages(ctx, req, resp); err != nil {
			return nil, err
		}
		return resp, nil
	}

	prepared, err := m.prepareRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := next.Call(ctx, prepared)
	if err != nil {
		return nil, err
	}

	if err := m.saveResponseMessages(ctx, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// executeStream is the streaming flow: prepare → stream chunks while
// accumulating → save the accumulated complete response after the
// stream closes naturally.
//
// Early consumer cancellation (caller breaks out of the iter loop)
// is treated as "abandon the turn": we do NOT persist the partial
// AssistantMessage, because half-streamed content saved as history
// would lie to the next turn about what the model actually said.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		resumed := isResumedTurn(ctx)

		// Continuation of a suspended turn (HITL resume): skip the input side
		// (already persisted; the tool loop resumes from its checkpoint and
		// ignores these request messages). A fresh turn loads history +
		// persists the new input here.
		prepared := req
		if !resumed {
			var err error
			prepared, err = m.prepareRequest(ctx, req)
			if err != nil {
				yield(nil, err)
				return
			}
		}

		// Accumulate the turn as an ORDERED message sequence, split on the
		// tool-message boundaries the tool loop emits between rounds, so each
		// round's assistant(tool_calls) is paired with its OWN tool results.
		// The merge-everything ResponseAccumulator collapses every round into
		// one assistant + keeps only the last round's tool message, which
		// orphans earlier tool_calls and makes the next turn's load an invalid
		// provider sequence ("'tool' must follow 'tool_calls'" / unanswered
		// calls).
		var seq []chat.Message
		round := chat.NewResponseAccumulator()
		flushRound := func() {
			if round.Response.Result != nil {
				if am := round.Response.Result.AssistantMessage; !blankAssistant(am) {
					seq = append(seq, am)
				}
			}
			round = chat.NewResponseAccumulator()
		}

		for chunk, err := range next.Stream(ctx, prepared) {
			if err != nil {
				yield(nil, err) // includes HITL interrupt — persist nothing
				return
			}
			// A tool message marks a round boundary: close the assistant that
			// requested it, then record the tool results right after it.
			if chunk != nil && chunk.Result != nil && chunk.Result.ToolMessage != nil {
				flushRound()
				seq = append(seq, chunk.Result.ToolMessage)
			} else {
				round.AddChunk(chunk)
			}
			if !yield(chunk, nil) {
				return // consumer canceled — skip persistence (abandon turn)
			}
		}
		flushRound() // the final assistant reply

		// On resume, prepend the turn's unsaved mid-flight tail (the rounds +
		// the assistant tool-call message the interrupt parked on, none
		// persisted mid-turn) so the tool results that follow keep their
		// preceding tool_calls.
		toSave := seq
		if resumed {
			toSave = append(m.filterUnsavedMessages(req.Messages), seq...)
		}
		if err := m.persistMessages(ctx, req, toSave...); err != nil {
			yield(nil, err)
		}
	}
}

// wrapCallHandler is the call-side adapter.
func (m *middleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

// wrapStreamHandler is the stream-side adapter.
func (m *middleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
