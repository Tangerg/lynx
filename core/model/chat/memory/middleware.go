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

// ConversationIDKey is the [chat.Request].Params key that identifies
// the conversation. Set it before calling the model:
//
//	req.Set(memory.ConversationIDKey, "session-42")
//
// When the key is absent the middleware short-circuits — no history
// load, no save — and the request passes through unchanged.
const ConversationIDKey = "lynx:ai:model:chat:memory:conversation_id"

// middleware is the conversation-memory layer. It owns exactly one job:
// on the way down it loads the stored history and splices it in front of
// the request's new messages; on the way up it persists the new messages
// plus the model's reply. It knows nothing about tool loops — each model
// round is one independent load → splice → call → save cycle.
//
// It is the INNERMOST middleware, sitting directly above the model: its
// `next` is the model handler, so the only reply it ever sees is one
// assistant message. The outer tool-calling middleware drives the loop and
// feeds each round's new messages (the user turn, then each tool result)
// down through here, so memory always receives genuinely-new messages and
// never needs de-duplication.
//
// Two invariants govern persistence and ordering:
//   - System messages are NEVER persisted. They are regenerated every turn
//     (they carry dynamic context) so storing them would duplicate them
//     across turns and pin a stale prompt.
//   - The system message is ALWAYS the first message sent to the model.
//     The spliced order is: system (from the live request) → stored
//     history → the request's new non-system messages.
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

// conversationID returns the conversation id stashed under
// [ConversationIDKey], or "" when the caller did not supply one.
// Returns an error if the value exists but is the wrong type.
func (m *middleware) conversationID(req *chat.Request) (string, error) {
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

// splice loads the conversation history and assembles the request sent to
// the model: the live request's system message first (never drawn from
// storage), then the stored history, then the request's new non-system
// messages. It also returns those new non-system messages — the slice to
// persist on the way up. Options / Params / Tools are cloned so the model
// sees an equivalent request shape.
func (m *middleware) splice(ctx context.Context, req *chat.Request, id string) (spliced *chat.Request, toPersist []chat.Message, err error) {
	history, err := m.store.Read(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	system := chat.FilterMessagesByMessageTypes(req.Messages, chat.MessageTypeSystem)
	fresh := chat.FilterMessagesByMessageTypes(req.Messages, chat.MessageTypeUser, chat.MessageTypeAssistant, chat.MessageTypeTool)

	combined := make([]chat.Message, 0, len(system)+len(history)+len(fresh))
	combined = append(combined, system...)
	combined = append(combined, history...)
	combined = append(combined, fresh...)

	next, err := chat.NewRequest(combined)
	if err != nil {
		return nil, nil, err
	}
	next.Options = req.Options.Clone()
	next.Params = maps.Clone(req.Params)
	next.Tools = slices.Clone(req.Tools)

	return next, fresh, nil
}

// persist writes the round's new input plus the model's reply under id, in
// order. System messages are excluded by construction (toPersist carries only
// the request's non-system messages).
//
// The reply is persisted ONLY when it is a final answer. A reply that requests
// tools is INCOMPLETE on its own: persisting it now would strand an
// assistant(tool_calls) with no answering tool message in the store if the
// turn then interrupts (HITL) or aborts before the tool runs — and a later
// turn loading that history would be rejected by the provider ("tool_calls
// must be followed by tool messages"). Instead the tool-calling middleware
// re-presents the assistant together with its tool result as the NEXT round's
// input, so the (assistant, tool) pair is written here atomically. A blank
// assistant — no text, no tool calls — is dropped so an empty round leaves no
// trace.
func (m *middleware) persist(ctx context.Context, id string, toPersist []chat.Message, resp *chat.Response) error {
	msgs := slices.Clone(toPersist)
	if resp != nil && resp.Result != nil {
		if am := resp.Result.AssistantMessage; !blankAssistant(am) && !am.HasToolCalls() {
			msgs = append(msgs, am)
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return m.store.Write(ctx, id, msgs...)
}

// blankAssistant reports whether an assistant message carries neither tool
// calls nor non-whitespace text — a round boundary with nothing to persist.
func blankAssistant(am *chat.AssistantMessage) bool {
	return am == nil || (!am.HasToolCalls() && strings.TrimSpace(am.JoinedText()) == "")
}

// executeCall is the synchronous flow: splice → call → save.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	id, err := m.conversationID(req)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return next.Call(ctx, req)
	}

	spliced, toPersist, err := m.splice(ctx, req, id)
	if err != nil {
		return nil, err
	}

	resp, err := next.Call(ctx, spliced)
	if err != nil {
		return nil, err
	}

	if err := m.persist(ctx, id, toPersist, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// executeStream is the streaming flow: splice → stream chunks while
// accumulating → save the accumulated reply after the stream closes
// naturally.
//
// Early consumer cancellation (the caller breaks out of the iter loop) and
// a stream error both abandon the round: we persist nothing, because a
// half-streamed reply saved as history would lie to the next turn about
// what the model actually said.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		id, err := m.conversationID(req)
		if err != nil {
			yield(nil, err)
			return
		}
		if id == "" {
			for resp, err := range next.Stream(ctx, req) {
				if !yield(resp, err) {
					return
				}
			}
			return
		}

		spliced, toPersist, err := m.splice(ctx, req, id)
		if err != nil {
			yield(nil, err)
			return
		}

		accumulator := chat.NewResponseAccumulator()
		for chunk, err := range next.Stream(ctx, spliced) {
			if err != nil {
				yield(nil, err) // stream error — persist nothing
				return
			}
			accumulator.AddChunk(chunk)
			if !yield(chunk, nil) {
				return // consumer canceled — persist nothing (abandon round)
			}
		}

		if err := m.persist(ctx, id, toPersist, &accumulator.Response); err != nil {
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
