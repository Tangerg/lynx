package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

// historyStore is the Runtime's consumer-side view of the chat-history service
// (one backing conversation.Messages). It is one cohesive interface, not
// per-verb sub-interfaces: the Runtime is the sole consumer and each method
// backs exactly one of the four history entry points below. It deliberately
// excludes the steering InjectUser path, which the turn dispatcher owns.
type historyStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Seed(ctx context.Context, sessionID string, msgs []chat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
	Truncate(ctx context.Context, sessionID string, keepN int) error
}

// ReadHistory returns sessionID's persisted chat history — the
// messages.list transport surface converts these to wire messages,
// and ForkSession copies a prefix of them.
func (r *Runtime) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return r.history.Read(ctx, sessionID)
}

// SeedHistory copies msgs into sessionID's chat history — used by
// ForkSession to seed a fresh child with the parent's prefix.
func (r *Runtime) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return r.history.Seed(ctx, sessionID, msgs)
}

// MessageCount returns sessionID's chat history message count — the per-run
// watermark sessions.rollback / fork record.
func (r *Runtime) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return r.history.Count(ctx, sessionID)
}

// TruncateMessages keeps the first keepN chat history messages of sessionID
// (sessions.rollback).
func (r *Runtime) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	return r.history.Truncate(ctx, sessionID, keepN)
}
