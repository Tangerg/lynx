// Package memory provides conversation-history primitives for chat
// applications. It defines the [Store] interface, two reference
// implementations ([InMemoryStore] and the windowed [MessageWindowStore]),
// and a middleware that auto-loads / auto-saves messages on every chat
// turn keyed by a conversation id.
//
// Example:
//
//	store := memory.NewInMemoryStore()
//	mw, _, err := memory.NewMiddleware(store)
//	resp, err := client.Chat().
//	    WithParams(map[string]any{memory.ConversationIDKey: "session-123"}).
//	    WithMiddlewares(mw).
//	    WithText("hi").
//	    Call().Response(ctx)
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

// Store is the union of [Reader], [Writer], and [Clearer]. It is the
// surface every memory backend implements; the framework treats it as
// "conversation context manager" rather than "complete chat history",
// so implementations are free to apply any retention strategy.
type Store interface {
	Reader
	Writer
	Clearer
}
