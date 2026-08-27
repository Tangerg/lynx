package inmemory

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
)

var (
	_ history.Store    = (*Store)(nil)
	_ history.Lister   = (*Store)(nil)
	_ history.Replacer = (*Store)(nil)
	_ history.Counter  = (*Store)(nil)
)

// Store is a concurrent in-process history store suitable for tests,
// development, and single-instance applications. Its zero value is ready to
// use. Writes and replacements validate then snapshot messages before locking;
// reads return deep caller-owned snapshots, and missing conversations behave as
// empty histories. Replace swaps the complete sequence atomically.
type Store struct {
	mu       sync.RWMutex
	messages map[history.ConversationID][]chat.Message
}

func New() *Store {
	return &Store{}
}

func (s *Store) Write(ctx context.Context, conversationID history.ConversationID, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := conversationID.Validate(); err != nil {
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
		s.messages = make(map[history.ConversationID][]chat.Message)
	}
	s.messages[conversationID] = append(s.messages[conversationID], messageSnapshot...)
	return nil
}

func (s *Store) Read(ctx context.Context, conversationID history.ConversationID) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := conversationID.Validate(); err != nil {
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

func (s *Store) Clear(ctx context.Context, conversationID history.ConversationID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := conversationID.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, conversationID)
	return nil
}

func (s *Store) Replace(ctx context.Context, conversationID history.ConversationID, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := conversationID.Validate(); err != nil {
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
		s.messages = make(map[history.ConversationID][]chat.Message)
	}
	s.messages[conversationID] = messageSnapshot
	return nil
}

func (s *Store) Count(ctx context.Context, conversationID history.ConversationID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := conversationID.Validate(); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[conversationID]), nil
}

func (s *Store) Conversations(ctx context.Context) ([]history.ConversationID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	ids := make([]history.ConversationID, 0, len(s.messages))
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
			return nil, fmt.Errorf("history/inmemory: messages[%d]: %w", index, err)
		}
		cloned[index] = messages[index].Clone()
	}
	return cloned, nil
}
