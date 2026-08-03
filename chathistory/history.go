package chathistory

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/internal/chathistorykit/nilcheck"
)

var (
	// ErrNilStore reports a helper called without a history store.
	ErrNilStore = errors.New("chathistory: nil store")
	// ErrReplacementUnsupported reports a Store without the optional atomic
	// replacement capability.
	ErrReplacementUnsupported = errors.New("chathistory: replacement unsupported")
)

// Reader returns the messages to replay for one conversation. Implementations
// return a non-nil empty slice for an unknown conversation and transfer
// ownership of returned protocol values to the caller.
type Reader interface {
	Read(ctx context.Context, conversationID string) ([]chat.Message, error)
}

// Writer appends messages to one conversation. Implementations preserve the
// order of messages within each call, validate and snapshot them before
// returning, and prevent later caller mutation from altering stored history.
// The relative order of concurrent calls and writes issued through distinct
// Store instances is implementation-defined.
type Writer interface {
	Write(ctx context.Context, conversationID string, messages ...chat.Message) error
}

// ReadWriter combines the capabilities required by components that replay and
// append history without owning retention or deletion policy.
type ReadWriter interface {
	Reader
	Writer
}

// Clearer removes every message for one conversation.
type Clearer interface {
	Clear(ctx context.Context, conversationID string) error
}

// Store is the ordinary per-conversation read/write/clear contract. Optional
// cross-conversation or retention capabilities remain separate interfaces.
type Store interface {
	ReadWriter
	Clearer
}

// Lister enumerates unique conversation IDs in lexical order. Implementations
// return a non-nil empty slice when no conversations exist. Concurrent
// mutations may affect the result.
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

// Replace uses store's optional atomic Replacer capability. It returns
// ErrReplacementUnsupported without modifying history when store does not
// implement Replacer.
func Replace(ctx context.Context, store Store, conversationID string, messages ...chat.Message) error {
	if nilcheck.IsNil(store) {
		return ErrNilStore
	}
	if replacer, ok := store.(Replacer); ok {
		return replacer.Replace(ctx, conversationID, messages...)
	}
	return ErrReplacementUnsupported
}

// Count uses store's optional Counter capability and otherwise falls back to
// reading the conversation.
func Count(ctx context.Context, store Store, conversationID string) (int, error) {
	if nilcheck.IsNil(store) {
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
