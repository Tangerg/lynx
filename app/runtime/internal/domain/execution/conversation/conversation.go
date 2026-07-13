// Package conversation is the LLM message-context domain: the
// chat.Message[] history fed to the model each turn, keyed by session.
// It wraps the chat history store (the same store the chat history
// middleware loads/saves) and owns the operations that read, seed,
// count, truncate, and inject into that history.
//
// This is one of the three distinct "histories" (see
// doc/EXECUTION_CENTERED_ARCHITECTURE.md): conversation (here) is what the LLM sees; knowledge is LYRA.md;
// transcript is the UI items+runs timeline. The engine drives turns; these
// messages own the out-of-turn history operations.
package conversation

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/history"
)

// errSessionIDRequired guards every operation: a session id is the history key,
// so an empty one is a programming error, not an empty history.
var errSessionIDRequired = errors.New("conversation: sessionID is required")

// Messages owns LLM message histories keyed by session over a chat history store.
type Messages struct {
	store history.Store
}

// NewMessages builds the message histories over store — the chat history
// backend (sqlite MessageStore in production, in-memory for tests). The chat
// history middleware loads/saves the same store during a turn; this type is the
// out-of-turn read/edit surface (fork, rollback, steering, messages.list).
func NewMessages(store history.Store) *Messages {
	return &Messages{store: store}
}

// Read returns sessionID's persisted message history — the same messages the
// chat history middleware loads at the start of each turn. Empty (nil, nil) for
// an unknown / never-used session.
func (m *Messages) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	if sessionID == "" {
		return nil, errSessionIDRequired
	}
	return m.store.Read(ctx, sessionID)
}

// Seed writes msgs into sessionID's history. Used by sessions.fork to copy a
// slice of the parent's history into a freshly created child so the child's
// next turn continues from the fork point. No-op for an empty slice. The store
// appends, so seed a fresh session only (seeding one with existing history
// would concatenate).
func (m *Messages) Seed(ctx context.Context, sessionID string, msgs []chat.Message) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if len(msgs) == 0 {
		return nil
	}
	return m.store.Write(ctx, sessionID, msgs...)
}

// Count returns sessionID's message count — the per-run watermark
// sessions.rollback / fork{fromRunId} record at segment.finished and truncate to.
// Empty session → 0.
func (m *Messages) Count(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, errSessionIDRequired
	}
	// history.Count uses the store's Counter capability (SQLite: SELECT COUNT(*))
	// when present, so this hot segment.finished watermark read doesn't load and
	// unmarshal the entire history just to count it; it falls back to len(Read)
	// for backends that can't count cheaply.
	return history.Count(ctx, m.store, sessionID)
}

// Truncate keeps the first keepN messages of sessionID and drops the rest
// (sessions.rollback). keepN >= current count is a no-op; keepN <= 0 clears the
// session. Store-agnostic — read the prefix, then atomically replace the
// history with it via [history.Replace], so a transactional backend can't be
// left wiped if the rewrite fails (the seq renumbering on re-write is
// immaterial; rollback doesn't depend on stable seqs).
func (m *Messages) Truncate(ctx context.Context, sessionID string, keepN int) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	msgs, err := m.store.Read(ctx, sessionID)
	if err != nil {
		return err
	}
	if keepN >= len(msgs) {
		return nil
	}
	// keepN <= 0 replaces with nothing, which clears the session.
	return history.Replace(ctx, m.store, sessionID, msgs[:max(keepN, 0)]...)
}

// InjectUser appends a synthetic user message to sessionID's history — it
// becomes part of the conversation the chat history middleware loads at the
// start of the next turn. The runtime uses this to deliver mid-turn steering
// once the current turn ends. Errors on an empty sessionID or text.
func (m *Messages) InjectUser(ctx context.Context, sessionID, text string) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if text == "" {
		return errors.New("conversation: text must not be empty")
	}
	return m.store.Write(ctx, sessionID, chat.NewUserMessage(text))
}
