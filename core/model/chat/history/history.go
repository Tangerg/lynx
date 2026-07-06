package history

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
// table scans — see chathistory/CLAUDE.md), whereas Conversations is an
// explicit cross-conversation scan for admin tasks (listing, bulk
// cleanup, migration, GC of orphaned sessions).
//
// Backends that can enumerate implement Lister; consumers reach for it
// via a type assertion, so a backend that cannot (or should not) scan
// still satisfies [Store]:
//
//	if lister, ok := store.(history.Lister); ok {
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

// Replacer atomically replaces a conversation's stored messages in one
// operation. Retention operations (truncation, compaction) need it: they
// read a conversation, derive a smaller rewrite, and persist it — a
// separate Clear then Write would LOSE the whole conversation if the Write
// failed after the Clear committed. Kept OUT of [Store] like [Lister]: it
// is not the per-turn hot path, and a backend that can't replace atomically
// still satisfies Store. Consumers reach for it via [Replace].
type Replacer interface {
	// Replace atomically sets conversationID's messages to exactly messages,
	// dropping any already stored. An empty messages clears the conversation.
	Replace(ctx context.Context, conversationID string, messages ...chat.Message) error
}

// Replace atomically replaces conversationID's messages via store's [Replacer]
// capability, falling back to a (non-atomic) Clear-then-Write when the backend
// doesn't implement it. Retention callers use this so an atomic backend never
// loses history to a rewrite that fails mid-way; a non-atomic backend keeps
// its best-effort behavior.
func Replace(ctx context.Context, store Store, conversationID string, messages ...chat.Message) error {
	if r, ok := store.(Replacer); ok {
		return r.Replace(ctx, conversationID, messages...)
	}
	if err := store.Clear(ctx, conversationID); err != nil {
		return err
	}
	return store.Write(ctx, conversationID, messages...)
}

// Counter reports a conversation's stored message count without materializing
// the messages. Kept OUT of [Store] like [Lister] / [Replacer]: counting is
// not the per-turn hot path, and a backend that can't count cheaply still
// satisfies Store. Consumers reach for it via [Count].
type Counter interface {
	// Count returns the number of messages stored for conversationID. An
	// unknown conversation is 0, not an error.
	Count(ctx context.Context, conversationID string) (int, error)
}

// Count returns conversationID's message count via store's [Counter]
// capability, falling back to len(Read) when the backend can't count without
// loading. Callers that need only the cardinality (e.g. a rollback watermark)
// use this so a backend able to do a cheap count — SELECT COUNT(*) — doesn't
// pay to materialize and unmarshal the whole history just to take its length.
func Count(ctx context.Context, store Store, conversationID string) (int, error) {
	if c, ok := store.(Counter); ok {
		return c.Count(ctx, conversationID)
	}
	msgs, err := store.Read(ctx, conversationID)
	if err != nil {
		return 0, err
	}
	return len(msgs), nil
}
