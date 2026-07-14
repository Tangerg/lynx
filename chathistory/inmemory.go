package chathistory

import (
	"context"
	"slices"
	"sort"
	"sync"

	"github.com/Tangerg/lynx/chathistory/internal/snapshot"
	"github.com/Tangerg/lynx/core/chat"
)

var (
	_ Store    = (*InMemoryStore)(nil)
	_ Lister   = (*InMemoryStore)(nil)
	_ Replacer = (*InMemoryStore)(nil)
	_ Counter  = (*InMemoryStore)(nil)
)

// InMemoryStore is a concurrent in-process Store suitable for tests,
// development, and single-instance applications. Its zero value is ready to
// use; NewInMemoryStore is provided for discoverability.
type InMemoryStore struct {
	mu       sync.RWMutex
	messages map[string][]chat.Message
}

// NewInMemoryStore returns an empty in-memory history store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{}
}

// Write validates, snapshots, and appends messages in order.
func (s *InMemoryStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateConversationID(conversationID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	messageSnapshot, err := snapshot.Messages(messages)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messages == nil {
		s.messages = make(map[string][]chat.Message)
	}
	s.messages[conversationID] = append(s.messages[conversationID], messageSnapshot...)
	return nil
}

// Read returns a deep caller-owned snapshot. Unknown IDs return a non-nil
// empty slice.
func (s *InMemoryStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateConversationID(conversationID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	stored := slices.Clone(s.messages[conversationID])
	s.mu.RUnlock()
	if len(stored) == 0 {
		return []chat.Message{}, nil
	}
	return snapshot.Messages(stored)
}

// Clear removes a conversation. Unknown IDs are ignored.
func (s *InMemoryStore) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateConversationID(conversationID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, conversationID)
	return nil
}

// Replace atomically swaps a conversation's complete message set.
func (s *InMemoryStore) Replace(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateConversationID(conversationID); err != nil {
		return err
	}
	messageSnapshot, err := snapshot.Messages(messages)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(messageSnapshot) == 0 {
		delete(s.messages, conversationID)
		return nil
	}
	if s.messages == nil {
		s.messages = make(map[string][]chat.Message)
	}
	s.messages[conversationID] = messageSnapshot
	return nil
}

// Count returns the stored cardinality without cloning message values.
func (s *InMemoryStore) Count(ctx context.Context, conversationID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := ValidateConversationID(conversationID); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[conversationID]), nil
}

// Conversations returns a sorted snapshot of all conversation IDs.
func (s *InMemoryStore) Conversations(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	ids := make([]string, 0, len(s.messages))
	for conversationID := range s.messages {
		ids = append(ids, conversationID)
	}
	s.mu.RUnlock()
	sort.Strings(ids)
	return ids, nil
}
