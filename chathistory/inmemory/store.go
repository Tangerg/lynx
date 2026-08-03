package inmemory

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

var (
	_ chathistory.Store    = (*Store)(nil)
	_ chathistory.Lister   = (*Store)(nil)
	_ chathistory.Replacer = (*Store)(nil)
	_ chathistory.Counter  = (*Store)(nil)
)

// Store is a concurrent in-process history store suitable for tests,
// development, and single-instance applications. Its zero value is ready to
// use; New is provided for discoverability.
type Store struct {
	mu       sync.RWMutex
	messages map[string][]chat.Message
}

// New returns an empty in-memory history store.
func New() *Store {
	return &Store{}
}

// Write validates, snapshots, and appends messages in order.
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	messageSnapshot, err := snapshotMessages(messages)
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
func (s *Store) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := chathistory.ValidateConversationID(conversationID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	stored := slices.Clone(s.messages[conversationID])
	s.mu.RUnlock()
	if len(stored) == 0 {
		return []chat.Message{}, nil
	}
	return snapshotMessages(stored)
}

// Clear removes a conversation. Unknown IDs are ignored.
func (s *Store) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, conversationID)
	return nil
}

// Replace atomically swaps a conversation's complete message set.
func (s *Store) Replace(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}
	messageSnapshot, err := snapshotMessages(messages)
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
func (s *Store) Count(ctx context.Context, conversationID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := chathistory.ValidateConversationID(conversationID); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[conversationID]), nil
}

// Conversations returns a sorted snapshot of all conversation IDs.
func (s *Store) Conversations(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	ids := make([]string, 0, len(s.messages))
	for conversationID := range s.messages {
		ids = append(ids, conversationID)
	}
	s.mu.RUnlock()
	slices.Sort(ids)
	return ids, nil
}

func snapshotMessages(messages []chat.Message) ([]chat.Message, error) {
	cloned := make([]chat.Message, len(messages))
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("chathistory/inmemory: messages[%d]: %w", index, err)
		}
		cloned[index] = messages[index].Clone()
	}
	return cloned, nil
}
