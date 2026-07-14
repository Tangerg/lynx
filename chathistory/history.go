package chathistory

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/chat"
)

// ErrNilStore reports a helper called without a history store.
var ErrNilStore = errors.New("chathistory: nil store")

// Reader returns the messages to replay for one conversation. Implementations
// return a non-nil empty slice for an unknown conversation and transfer
// ownership of returned protocol values to the caller.
type Reader interface {
	Read(ctx context.Context, conversationID string) ([]chat.Message, error)
}

// Writer appends messages to one conversation. Implementations validate and
// snapshot messages before returning so later caller mutation cannot alter
// stored history.
type Writer interface {
	Write(ctx context.Context, conversationID string, messages ...chat.Message) error
}

// Clearer removes every message for one conversation.
type Clearer interface {
	Clear(ctx context.Context, conversationID string) error
}

// Store is the ordinary per-conversation read/write/clear contract. Optional
// cross-conversation or retention capabilities remain separate interfaces.
type Store interface {
	Reader
	Writer
	Clearer
}

// Lister enumerates conversation IDs as a point-in-time snapshot. Ordering is
// implementation-defined.
type Lister interface {
	Conversations(ctx context.Context) ([]string, error)
}

// Replacer atomically sets a conversation's messages to exactly messages.
type Replacer interface {
	Replace(ctx context.Context, conversationID string, messages ...chat.Message) error
}

// Counter reports a conversation's stored message count without requiring
// callers to materialize its messages.
type Counter interface {
	Count(ctx context.Context, conversationID string) (int, error)
}

// Replace uses store's optional atomic Replacer capability. Stores without it
// fall back to clear-then-write and therefore cannot promise atomicity.
func Replace(ctx context.Context, store Store, conversationID string, messages ...chat.Message) error {
	if store == nil {
		return ErrNilStore
	}
	if replacer, ok := store.(Replacer); ok {
		return replacer.Replace(ctx, conversationID, messages...)
	}
	if err := store.Clear(ctx, conversationID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	return store.Write(ctx, conversationID, messages...)
}

// Count uses store's optional Counter capability and otherwise falls back to
// reading the conversation.
func Count(ctx context.Context, store Store, conversationID string) (int, error) {
	if store == nil {
		return 0, ErrNilStore
	}
	if counter, ok := store.(Counter); ok {
		return counter.Count(ctx, conversationID)
	}
	messages, err := store.Read(ctx, conversationID)
	if err != nil {
		return 0, err
	}
	return len(messages), nil
}
