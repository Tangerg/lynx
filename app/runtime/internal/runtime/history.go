package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

// historyStore is the Runtime's consumer-side view of the chat-history service
// (one backing conversation.Messages): the read projection the messages.list
// surface converts to wire, and the per-run message-count watermark the run
// boundary records. History WRITES (seed/truncate) belong to the sessions
// coordinator's atomic write-sets, not here.
type historyStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Count(ctx context.Context, sessionID string) (int, error)
}

// ReadHistory returns sessionID's persisted chat history — the messages.list
// transport surface converts these to wire messages.
func (r *Runtime) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return r.history.Read(ctx, sessionID)
}

// MessageCount returns sessionID's chat history message count — the per-run
// watermark sessions.rollback / fork record.
func (r *Runtime) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return r.history.Count(ctx, sessionID)
}
