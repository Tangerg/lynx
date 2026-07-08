package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

type historyReader interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
}

type historySeeder interface {
	Seed(ctx context.Context, sessionID string, msgs []chat.Message) error
}

type historyCounter interface {
	Count(ctx context.Context, sessionID string) (int, error)
}

type historyTruncator interface {
	Truncate(ctx context.Context, sessionID string, keepN int) error
}

// ReadHistory returns sessionID's persisted chat history — the
// messages.list transport surface converts these to wire messages,
// and ForkSession copies a prefix of them.
func (r *Runtime) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return r.historyRead.Read(ctx, sessionID)
}

// SeedHistory copies msgs into sessionID's chat history — used by
// ForkSession to seed a fresh child with the parent's prefix.
func (r *Runtime) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return r.historySeed.Seed(ctx, sessionID, msgs)
}

// MessageCount returns sessionID's chat history message count — the per-run
// watermark sessions.rollback / fork record.
func (r *Runtime) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return r.historyCount.Count(ctx, sessionID)
}

// TruncateMessages keeps the first keepN chat history messages of sessionID
// (sessions.rollback).
func (r *Runtime) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	return r.historyTruncate.Truncate(ctx, sessionID, keepN)
}
