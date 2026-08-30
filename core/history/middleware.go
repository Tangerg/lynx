package history

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
)

var ErrNilStream = errors.New("history: middleware: nil stream sequence")

// Middleware replays and persists history around synchronous and streaming
// chat capabilities. It is immutable after construction and safe for
// concurrent use when its Store is safe for concurrent use.
type Middleware struct {
	store ReadWriter
}

func NewMiddleware(store ReadWriter) (Middleware, error) {
	if lo.IsNil(store) {
		return Middleware{}, ErrNilStore
	}
	return Middleware{store: store}, nil
}

// Call is a [chat.CallMiddleware]. The response result is the canonical
// assistant message persisted to history.
func (m Middleware) Call(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		conversationID, bound := ConversationIDFromContext(ctx)
		if !bound {
			return next.Call(ctx, request)
		}

		prepared, fresh, err := m.prepare(ctx, conversationID, request)
		if err != nil {
			return nil, err
		}
		response, err := next.Call(ctx, prepared)
		if err != nil {
			return response, err
		}
		assistant, persist := m.persistableAssistant(response)
		if !persist {
			return response, nil
		}
		if err := m.persist(ctx, conversationID, fresh, assistant); err != nil {
			return response, err
		}
		return response, nil
	})
}

// Stream is a [chat.StreamMiddleware]. History I/O remains lazy: no read occurs
// until the returned sequence is iterated. Fresh input and the accumulated
// assistant response are persisted only after natural, error-free completion.
func (m Middleware) Stream(next chat.Streamer) chat.Streamer {
	return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			m.stream(ctx, next, request, yield)
		}
	})
}

func (m Middleware) stream(
	ctx context.Context,
	next chat.Streamer,
	request *chat.Request,
	yield func(*chat.Response, error) bool,
) {
	conversationID, bound := ConversationIDFromContext(ctx)
	if !bound {
		m.forward(next.Stream(ctx, request), yield)
		return
	}

	prepared, fresh, err := m.prepare(ctx, conversationID, request)
	if err != nil {
		yield(nil, err)
		return
	}
	sequence := next.Stream(ctx, prepared)
	if sequence == nil {
		yield(nil, ErrNilStream)
		return
	}

	stream := historyStream{yield: yield, natural: true}
	sequence(stream.consume)
	if !stream.natural {
		return
	}
	assistant, persist := m.persistableAssistant(stream.accumulator.Response())
	if !persist {
		return
	}
	if err := m.persist(ctx, conversationID, fresh, assistant); err != nil {
		yield(nil, err)
	}
}

type historyStream struct {
	yield       func(*chat.Response, error) bool
	accumulator chat.ResponseAccumulator
	natural     bool
}

func (h *historyStream) consume(chunk *chat.Response, streamErr error) bool {
	if !h.natural {
		return false
	}
	if streamErr != nil {
		return h.stop(chunk, streamErr)
	}
	if err := h.accumulator.Add(chunk); err != nil {
		return h.stop(nil, fmt.Errorf("history: middleware: accumulate stream: %w", err))
	}
	if !h.yield(chunk, nil) {
		h.natural = false
		return false
	}
	return true
}

func (h *historyStream) stop(response *chat.Response, err error) bool {
	h.natural = false
	h.yield(response, err)
	return false
}

func (m Middleware) prepare(
	ctx context.Context,
	conversationID ConversationID,
	request *chat.Request,
) (*chat.Request, []chat.Message, error) {
	if err := conversationID.Validate(); err != nil {
		return nil, nil, err
	}
	prepared, err := m.snapshotRequest(request)
	if err != nil {
		return nil, nil, fmt.Errorf("history: middleware: prepare request: %w", err)
	}
	stored, err := m.store.Read(ctx, conversationID)
	if err != nil {
		return nil, nil, fmt.Errorf("history: middleware: read history: %w", err)
	}
	stored, err = historyMessages(stored).snapshot()
	if err != nil {
		return nil, nil, fmt.Errorf("history: middleware: validate stored history: %w", err)
	}

	systems, fresh := historyMessages(prepared.Messages).split()
	freshSnapshot, err := fresh.snapshot()
	if err != nil {
		return nil, nil, fmt.Errorf("history: middleware: snapshot fresh messages: %w", err)
	}
	prepared.Messages = make([]chat.Message, 0, len(systems)+len(stored)+len(fresh))
	prepared.Messages = append(prepared.Messages, systems...)
	for _, message := range stored {
		if message.Role != chat.RoleSystem {
			prepared.Messages = append(prepared.Messages, message)
		}
	}
	prepared.Messages = append(prepared.Messages, fresh...)
	if err := prepared.Validate(); err != nil {
		return nil, nil, fmt.Errorf("history: middleware: assemble request history: %w", err)
	}
	return prepared, freshSnapshot, nil
}

func (m Middleware) persist(
	ctx context.Context,
	conversationID ConversationID,
	fresh []chat.Message,
	assistant chat.Message,
) error {
	messages := make([]chat.Message, 0, len(fresh)+1)
	messages = append(messages, fresh...)
	messages = append(messages, assistant)
	if err := m.store.Write(ctx, conversationID, messages...); err != nil {
		return fmt.Errorf("history: middleware: write history: %w", err)
	}
	return nil
}

func (m Middleware) snapshotRequest(request *chat.Request) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return request.Clone(), nil
}

func (m Middleware) persistableAssistant(response *chat.Response) (chat.Message, bool) {
	if response == nil || response.Output == nil || response.Output.Message == nil || response.Output.Message.Role != chat.RoleAssistant {
		return chat.Message{}, false
	}
	for _, part := range response.Output.Message.Parts {
		if part.Kind == chat.PartToolCall {
			return chat.Message{}, false
		}
	}
	return response.Output.Message.Clone(), true
}

func (m Middleware) forward(sequence iter.Seq2[*chat.Response, error], yield func(*chat.Response, error) bool) {
	if sequence == nil {
		yield(nil, ErrNilStream)
		return
	}
	sequence(yield)
}

type historyMessages []chat.Message

func (h historyMessages) split() (systems, nonSystems historyMessages) {
	systems = make([]chat.Message, 0, len(h))
	nonSystems = make([]chat.Message, 0, len(h))
	for _, message := range h {
		if message.Role == chat.RoleSystem {
			systems = append(systems, message)
		} else {
			nonSystems = append(nonSystems, message)
		}
	}
	return systems, nonSystems
}

func (h historyMessages) snapshot() ([]chat.Message, error) {
	cloned := make([]chat.Message, len(h))
	for index := range h {
		if err := h[index].Validate(); err != nil {
			return nil, fmt.Errorf("history: messages[%d]: %w", index, err)
		}
		cloned[index] = h[index].Clone()
	}
	return cloned, nil
}
