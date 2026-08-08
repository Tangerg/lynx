// Package conversations owns use cases over durable model-context histories.
package conversations

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/core/chat"
)

var errSessionIDRequired = errors.New("conversations: session ID is required")

// Store is the exact persistence capability consumed by conversation use
// cases. Replace must atomically install the complete sequence.
type Store interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Write(ctx context.Context, sessionID string, messages ...chat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
	Replace(ctx context.Context, sessionID string, messages ...chat.Message) error
}

// Messages coordinates durable conversation operations while the domain value
// owns sequence validation and transformations.
type Messages struct {
	store Store
}

// NewMessages returns the conversation use cases backed by store.
func NewMessages(store Store) *Messages { return &Messages{store: store} }

// Read returns the validated durable conversation snapshot.
func (m *Messages) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	if sessionID == "" {
		return nil, errSessionIDRequired
	}
	messages, err := m.store.Read(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("conversations: read session %q: %w", sessionID, err)
	}
	history, err := conversation.New(messages)
	if err != nil {
		return nil, fmt.Errorf("conversations: read session %q: %w", sessionID, err)
	}
	return history.Messages(), nil
}

// Seed installs a prefix into a fresh conversation. Existing history is never
// silently appended to or replaced by a fork/import operation.
func (m *Messages) Seed(ctx context.Context, sessionID string, messages []chat.Message) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if len(messages) == 0 {
		return nil
	}
	count, err := m.store.Count(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("conversations: inspect seed target %q: %w", sessionID, err)
	}
	current := conversation.Conversation{}
	if count != 0 {
		return conversation.ErrNotEmpty
	}
	seeded, err := current.Seed(messages)
	if err != nil {
		return err
	}
	if err := m.store.Write(ctx, sessionID, seeded.Messages()...); err != nil {
		return fmt.Errorf("conversations: seed session %q: %w", sessionID, err)
	}
	return nil
}

// Count returns the durable message watermark.
func (m *Messages) Count(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, errSessionIDRequired
	}
	count, err := m.store.Count(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("conversations: count session %q: %w", sessionID, err)
	}
	return count, nil
}

// Truncate atomically keeps the first keepN messages.
func (m *Messages) Truncate(ctx context.Context, sessionID string, keepN int) error {
	stored, err := m.Read(ctx, sessionID)
	if err != nil {
		return err
	}
	history, err := conversation.New(stored)
	if err != nil {
		return err
	}
	if keepN >= history.Count() {
		return nil
	}
	if err := m.store.Replace(ctx, sessionID, history.Truncate(keepN).Messages()...); err != nil {
		return fmt.Errorf("conversations: truncate session %q to %d messages: %w", sessionID, max(keepN, 0), err)
	}
	return nil
}

// Clear atomically removes every message without first decoding stored rows.
func (m *Messages) Clear(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if err := m.store.Replace(ctx, sessionID); err != nil {
		return fmt.Errorf("conversations: clear session %q: %w", sessionID, err)
	}
	return nil
}

// AppendUserMessage validates and appends one user message.
func (m *Messages) AppendUserMessage(ctx context.Context, sessionID string, message chat.Message) error {
	if sessionID == "" {
		return errSessionIDRequired
	}
	if _, err := (conversation.Conversation{}).AppendUser(message); err != nil {
		return err
	}
	if err := m.store.Write(ctx, sessionID, message.Clone()); err != nil {
		return fmt.Errorf("conversations: append user message to session %q: %w", sessionID, err)
	}
	return nil
}
