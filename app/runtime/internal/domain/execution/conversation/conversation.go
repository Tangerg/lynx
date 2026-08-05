// Package conversation is the LLM message-context domain: the
// chat.Message[] history fed to model executions, keyed by Session.
// It wraps the same persistence the chat-history middleware loads and saves,
// and owns the operations that read, seed,
// count, truncate, and inject into that history.
//
// This is one of the three distinct "histories" (see
// doc/EXECUTION_CENTERED_ARCHITECTURE.md): conversation is model context,
// knowledge is LYRA.md, and transcript is the observable Items-and-Runs record.
// Execution drives Runs; this package owns message history outside active
// execution.
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
	errSessionIDRequired   = errors.New("conversation: session ID is required")
	errUserMessageRequired = errors.New("conversation: valid user message is required")
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

// NewMessages builds message histories over store. Active execution and
// independent fork, rollback, steering, and read operations share this history.
func NewMessages(store Store) *Messages {
	return &Messages{store: store}
}

// Read returns sessionID's persisted message history — the same messages the
// chat history middleware loads at the start of each execution. Empty (nil, nil) for
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

// Seed writes messages into sessionID's history. Fork uses it to copy a
// slice of the parent's history into a freshly created child so the child's
// next execution continues from the fork point. No-op for an empty slice. The store
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
// rollback or fork boundary records at segment completion and truncate to.
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
// during rollback. keepN >= current count is a no-op; keepN <= 0 clears the
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

// Clear removes every message for sessionID. Unlike Truncate it does not read the
// history first, so it cannot be short-circuited into a no-op by a session whose
// rows are unparseable (Read skips malformed rows) — those would otherwise survive
// a "clear" as orphans keyed to a deleted session. Use this for a full wipe (delete
// / full-history reset); Truncate stays for keeping a prefix.
func (m *Messages) Clear(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if err := m.store.Replace(ctx, sessionID); err != nil {
		return fmt.Errorf("conversation: clear session %q: %w", sessionID, err)
	}
	return nil
}

// AppendUserMessage appends a validated user message to sessionID's history. It
// becomes part of the conversation the chat history middleware loads at the
// start of the next execution. This preserves structured steering content that
// misses the current execution's final continuation round.
func (m *Messages) AppendUserMessage(ctx context.Context, sessionID string, message chat.Message) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if message.Role != chat.RoleUser {
		return errUserMessageRequired
	}
	if err := message.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errUserMessageRequired, err)
	}
	if err := m.store.Write(ctx, sessionID, message); err != nil {
		return fmt.Errorf("conversation: append user message to session %q: %w", sessionID, err)
	}
	return nil
}
