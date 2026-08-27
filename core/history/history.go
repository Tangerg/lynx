package history

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
)

var (
	ErrNilStore               = errors.New("history: nil store")
	ErrReplacementUnsupported = errors.New("history: replacement unsupported")
)

// Reader returns the messages to replay for one conversation. Implementations
// return a non-nil empty slice for an unknown conversation and transfer
// ownership of returned protocol values to the caller.
type Reader interface {
	// Read returns a detached snapshot in stored order. Unknown conversations
	// yield a non-nil empty slice; implementations honor ctx and never expose
	// mutable backing storage.
	Read(ctx context.Context, conversationID ConversationID) ([]chat.Message, error)
}

// Writer appends messages to one conversation. Implementations preserve the
// order of messages within each call, validate and snapshot them before
// returning, and prevent later caller mutation from altering stored history.
// The relative order of concurrent calls and writes issued through distinct
// Store instances is implementation-defined.
type Writer interface {
	// Write validates and snapshots the full argument batch before appending it
	// in argument order. A returned error must not conceal a partially accepted
	// prefix unless the concrete store documents an external atomicity limit.
	Write(ctx context.Context, conversationID ConversationID, messages ...chat.Message) error
}

// ReadWriter combines the capabilities required by components that replay and
// append history without owning retention or deletion policy.
type ReadWriter interface {
	Reader
	Writer
}

// Clearer removes every message for one conversation.
type Clearer interface {
	// Clear removes the complete conversation and is idempotent when it is
	// already absent. Implementations must honor ctx.
	Clear(ctx context.Context, conversationID ConversationID) error
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
	// Conversations returns detached, unique identifiers in lexical order. An
	// empty store yields a non-nil empty slice; concurrent writes may appear or
	// not according to the backend's snapshot boundary.
	Conversations(ctx context.Context) ([]ConversationID, error)
}

// Replacer atomically sets a conversation's messages to exactly messages.
type Replacer interface {
	// Replace atomically swaps one conversation for the validated message batch.
	// An empty batch leaves the conversation present but empty; ctx bounds the
	// backend operation.
	Replace(ctx context.Context, conversationID ConversationID, messages ...chat.Message) error
}

// Counter reports a conversation's stored message count without requiring
// callers to materialize its messages.
type Counter interface {
	// Count returns the number of messages visible at one backend read boundary
	// without materializing them. Unknown conversations return zero.
	Count(ctx context.Context, conversationID ConversationID) (int, error)
}

// Replace uses store's optional atomic Replacer capability. It returns
// ErrReplacementUnsupported without modifying history when store does not
// implement Replacer.
func Replace(ctx context.Context, store Store, conversationID ConversationID, messages ...chat.Message) error {
	if lo.IsNil(store) {
		return ErrNilStore
	}
	if replacer, ok := store.(Replacer); ok {
		return replacer.Replace(ctx, conversationID, messages...)
	}
	return ErrReplacementUnsupported
}

// Count uses store's optional Counter capability and otherwise falls back to
// reading the conversation.
func Count(ctx context.Context, store Store, conversationID ConversationID) (int, error) {
	if lo.IsNil(store) {
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
