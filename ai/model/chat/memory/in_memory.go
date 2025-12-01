package memory

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ Store = (*InMemoryStore)(nil)

// InMemoryStore is an in-memory implementation of Store.
// It stores chat messages using a map with read-write mu for thread safety.
// This implementation is suitable for development and testing environments
// but does not persist data across application restarts.
type InMemoryStore struct {
	mu    sync.RWMutex
	store map[string][]chat.Message
}

func NewInMemoryMemory() *InMemoryStore {
	return &InMemoryStore{
		store: make(map[string][]chat.Message),
	}
}

// Write stores the specified messages for the given conversation ID.
// If no messages are provided, the operation is a no-op.
func (m *InMemoryStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.store[conversationID] = append(m.store[conversationID], messages...)
	return nil
}

// Read retrieves all stored messages for the specified conversation ID.
// Returns an empty slice if the conversation ID does not exist.
func (m *InMemoryStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	stored, exists := m.store[conversationID]
	if !exists {
		return []chat.Message{}, nil
	}

	// Return a copy to prevent external modification
	copied := make([]chat.Message, len(stored))
	copy(copied, stored)
	return copied, nil
}

// Clear removes all stored messages for the specified conversation ID.
func (m *InMemoryStore) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.store, conversationID)
	return nil
}
