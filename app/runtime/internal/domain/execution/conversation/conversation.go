// Package conversation is the LLM message-context domain: the
// chat.Message[] history fed to the model each turn, keyed by session.
// It wraps the same persistence the chat-history middleware loads and saves,
// and owns the operations that read, seed,
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
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

var (
	// errSessionIDRequired guards every operation: a session ID is the history
	// key, so an empty one is a programming error, not an empty history.
	errSessionIDRequired = errors.New("conversation: session ID is required")
	errTextRequired      = errors.New("conversation: text must not be empty")
)

// Store is the conversation domain's persistence port. Replace must set the
// complete history atomically: rollback and restore may never expose a cleared
// or partially rewritten conversation. Count is required because run-boundary
// watermarks are part of the domain contract, not an optional optimization.
type Store interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Write(ctx context.Context, sessionID string, messages ...chat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
	Replace(ctx context.Context, sessionID string, messages ...chat.Message) error
}

// Messages owns LLM message histories keyed by session over a chat history store.
type Messages struct {
	store Store
}

// NewMessages builds the message histories over store — the chat history
// backend (sqlite MessageStore in production, in-memory for tests). The chat
// history middleware loads/saves the same store during a turn; this type is the
// out-of-turn read/edit surface (fork, rollback, steering, messages.list).
func NewMessages(store Store) *Messages {
	return &Messages{store: store}
}

// Read returns sessionID's persisted message history — the same messages the
// chat history middleware loads at the start of each turn. Empty (nil, nil) for
// an unknown / never-used session.
func (m *Messages) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	if sessionID == "" {
		return nil, errSessionIDRequired
	}
	messages, err := m.store.Read(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("conversation: read session %q: %w", sessionID, err)
	}
	return messages, nil
}

// Seed writes messages into sessionID's history. Used by sessions.fork to copy a
// slice of the parent's history into a freshly created child so the child's
// next turn continues from the fork point. No-op for an empty slice. The store
// appends, so seed a fresh session only (seeding one with existing history
// would concatenate).
func (m *Messages) Seed(ctx context.Context, sessionID string, messages []chat.Message) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if len(messages) == 0 {
		return nil
	}
	if err := m.store.Write(ctx, sessionID, messages...); err != nil {
		return fmt.Errorf("conversation: seed session %q: %w", sessionID, err)
	}
	return nil
}

// Count returns sessionID's message count — the per-run watermark
// sessions.rollback / fork{fromRunId} record at segment.finished and truncate to.
// Empty session → 0.
func (m *Messages) Count(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, errSessionIDRequired
	}
	count, err := m.store.Count(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("conversation: count session %q: %w", sessionID, err)
	}
	return count, nil
}

// Truncate keeps the first keepN messages of sessionID and drops the rest
// (sessions.rollback). keepN >= current count is a no-op; keepN <= 0 clears the
// session. It reads the prefix and atomically replaces the history through the
// required [Store] contract, so a failed rewrite leaves the prior history
// intact (sequence renumbering is immaterial; rollback does not depend on it).
func (m *Messages) Truncate(ctx context.Context, sessionID string, keepN int) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	stored, err := m.Read(ctx, sessionID)
	if err != nil {
		return err
	}
	if keepN >= len(stored) {
		return nil
	}
	// keepN <= 0 replaces with nothing, which clears the session.
	if err := m.store.Replace(ctx, sessionID, stored[:max(keepN, 0)]...); err != nil {
		return fmt.Errorf("conversation: truncate session %q to %d messages: %w", sessionID, max(keepN, 0), err)
	}
	return nil
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
		return errTextRequired
	}
	if err := m.store.Write(ctx, sessionID, chat.NewUserMessage(chat.NewTextPart(text))); err != nil {
		return fmt.Errorf("conversation: inject user message into session %q: %w", sessionID, err)
	}
	return nil
}
