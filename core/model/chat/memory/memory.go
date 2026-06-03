package memory

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Reader returns the messages for a conversation that should be sent to
// the LLM as context.
type Reader interface {
	// Read returns the contextually relevant messages for conversationID.
	// What "relevant" means is up to the implementation — sliding window,
	// token budget, prioritized summary, etc.
	Read(ctx context.Context, conversationID string) ([]chat.Message, error)
}

// Writer appends new messages to a conversation.
type Writer interface {
	// Write appends messages to conversationID. Implementations may apply
	// retention rules (eviction, summarization, ...) at write time.
	Write(ctx context.Context, conversationID string, messages ...chat.Message) error
}

// Clearer drops every message for a conversation.
type Clearer interface {
	// Clear removes all stored messages for conversationID.
	Clear(ctx context.Context, conversationID string) error
}

// Lister enumerates the conversations a backend holds. It is an
// optional, ops-oriented capability deliberately kept OUT of [Store]:
// the hot read/write path stays partitioned by conversation id (no
// table scans — see chatmemory/CLAUDE.md), whereas Conversations is an
// explicit cross-conversation scan for admin tasks (listing, bulk
// cleanup, migration, GC of orphaned sessions).
//
// Backends that can enumerate implement Lister; consumers reach for it
// via a type assertion, so a backend that cannot (or should not) scan
// still satisfies [Store]:
//
//	if lister, ok := store.(memory.Lister); ok {
//	    ids, err := lister.Conversations(ctx)
//	}
type Lister interface {
	// Conversations returns the ids of every stored conversation, in no
	// guaranteed order — a point-in-time snapshot. Implementations honor
	// ctx cancellation since this may scan the whole keyspace.
	Conversations(ctx context.Context) ([]string, error)
}

// Store is the union of [Reader], [Writer], and [Clearer]. It is the
// surface every memory backend implements; the framework treats it as
// "conversation context manager" rather than "complete chat history",
// so implementations are free to apply any retention strategy.
//
// Enumeration is intentionally not part of Store — see [Lister].
type Store interface {
	Reader
	Writer
	Clearer
}
