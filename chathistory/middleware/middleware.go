package middleware

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/internal/snapshot"
	"github.com/Tangerg/lynx/core/chat"
)

// ErrNilStream reports a wrapped Streamer that violates the Streamer contract
// by returning a nil sequence.
var ErrNilStream = errors.New("chathistory middleware: nil stream sequence")

// Store is the consumer-side history capability required by Middleware.
// Clear, list, retention, and backend-specific operations stay outside this
// dependency boundary.
type Store interface {
	chathistory.Reader
	chathistory.Writer
}

// Middleware replays and persists history around synchronous and streaming
// chat capabilities. It is immutable after construction and safe for
// concurrent use when its Store is safe for concurrent use.
type Middleware struct {
	store Store
}

// New constructs history middleware around store.
func New(store Store) (*Middleware, error) {
	if store == nil {
		return nil, chathistory.ErrNilStore
	}
	return &Middleware{store: store}, nil
}

// Call is a [chat.CallMiddleware]. The first response choice is the canonical
// history choice, matching [chat.Response.First] and [chat.Response.Text].
func (m *Middleware) Call(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		conversationID, bound := chathistory.ConversationID(ctx)
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
		assistant, persist := persistableAssistant(response)
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
func (m *Middleware) Stream(next chat.Streamer) chat.Streamer {
	return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			conversationID, bound := chathistory.ConversationID(ctx)
			if !bound {
				forward(next.Stream(ctx, request), yield)
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

			var accumulator chat.ResponseAccumulator
			natural := true
			sequence(func(chunk *chat.Response, streamErr error) bool {
				if !natural {
					return false
				}
				if streamErr != nil {
					natural = false
					yield(chunk, streamErr)
					return false
				}
				if err := accumulator.Add(chunk); err != nil {
					natural = false
					yield(nil, fmt.Errorf("chathistory middleware: accumulate stream: %w", err))
					return false
				}
				if !yield(chunk, nil) {
					natural = false
					return false
				}
				return true
			})
			if !natural {
				return
			}

			assistant, persist := persistableAssistant(accumulator.Response())
			if !persist {
				return
			}
			if err := m.persist(ctx, conversationID, fresh, assistant); err != nil {
				yield(nil, err)
			}
		}
	})
}

func (m *Middleware) prepare(
	ctx context.Context,
	conversationID string,
	request *chat.Request,
) (*chat.Request, []chat.Message, error) {
	if err := chathistory.ValidateConversationID(conversationID); err != nil {
		return nil, nil, err
	}
	prepared, err := snapshot.Request(request)
	if err != nil {
		return nil, nil, err
	}
	stored, err := m.store.Read(ctx, conversationID)
	if err != nil {
		return nil, nil, fmt.Errorf("chathistory middleware: read: %w", err)
	}
	stored, err = snapshot.Messages(stored)
	if err != nil {
		return nil, nil, fmt.Errorf("chathistory middleware: invalid stored history: %w", err)
	}

	systems, fresh := splitMessages(prepared.Messages)
	freshSnapshot, err := snapshot.Messages(fresh)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}
	return prepared, freshSnapshot, nil
}

func (m *Middleware) persist(
	ctx context.Context,
	conversationID string,
	fresh []chat.Message,
	assistant chat.Message,
) error {
	messages := make([]chat.Message, 0, len(fresh)+1)
	messages = append(messages, fresh...)
	messages = append(messages, assistant)
	if err := m.store.Write(ctx, conversationID, messages...); err != nil {
		return fmt.Errorf("chathistory middleware: write: %w", err)
	}
	return nil
}

func splitMessages(messages []chat.Message) (systems, nonSystems []chat.Message) {
	systems = make([]chat.Message, 0, len(messages))
	nonSystems = make([]chat.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == chat.RoleSystem {
			systems = append(systems, message)
		} else {
			nonSystems = append(nonSystems, message)
		}
	}
	return systems, nonSystems
}

func persistableAssistant(response *chat.Response) (chat.Message, bool) {
	choice := response.First()
	if choice == nil || choice.Message == nil || choice.Message.Role != chat.RoleAssistant {
		return chat.Message{}, false
	}
	for _, part := range choice.Message.Parts {
		if part.Kind == chat.PartToolCall {
			return chat.Message{}, false
		}
	}
	return snapshot.Message(*choice.Message), true
}

func forward(sequence iter.Seq2[*chat.Response, error], yield func(*chat.Response, error) bool) {
	if sequence == nil {
		yield(nil, ErrNilStream)
		return
	}
	sequence(yield)
}
