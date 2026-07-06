package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ReadHistory returns sessionID's persisted chat history — the
// messages.list transport surface converts these to wire messages,
// and ForkSession copies a prefix of them.
func (r *Runtime) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return r.conversation.Read(ctx, sessionID)
}

// SeedHistory copies msgs into sessionID's chat history — used by
// ForkSession to seed a fresh child with the parent's prefix.
func (r *Runtime) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return r.conversation.Seed(ctx, sessionID, msgs)
}

// MessageCount returns sessionID's chat history message count — the per-run
// watermark sessions.rollback / fork record.
func (r *Runtime) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return r.conversation.Count(ctx, sessionID)
}

// TruncateMessages keeps the first keepN chat history messages of sessionID
// (sessions.rollback).
func (r *Runtime) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	return r.conversation.Truncate(ctx, sessionID, keepN)
}
