package runtime

import "context"

// historyStore is the Runtime's consumer-side view of the chat-history service
// (one backing conversation.Messages): only the per-run message-count watermark
// the run boundary records. History reads (messages.list) are an application
// query and writes (seed/truncate) are the sessions coordinator's write-sets —
// neither lives here.
type historyStore interface {
	Count(ctx context.Context, sessionID string) (int, error)
}

// MessageCount returns sessionID's chat history message count — the per-run
// watermark sessions.rollback / fork record.
func (r *Runtime) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return r.history.Count(ctx, sessionID)
}
