package memories

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ chat.Memory = (*InMemoryMemory)(nil)

// InMemoryMemory is an in-memory implementation of chat.Memory.
// It stores chat messages using a map with read-write mutex for thread safety.
// This implementation is suitable for development and testing environments
// but does not persist data across application restarts.
type InMemoryMemory struct {
	mu    sync.RWMutex
	store map[string][]chat.Message
}

// NewInMemoryMemory creates a new instance of InMemoryMemory.
func NewInMemoryMemory() *InMemoryMemory {
	return &InMemoryMemory{
		store: make(map[string][]chat.Message),
	}
}

// Write stores the specified messages for the given conversation ID.
// If no messages are provided, the operation is a no-op.
func (m *InMemoryMemory) Write(ctx context.Context, conversationID string, msgs ...chat.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.store[conversationID] = append(m.store[conversationID], msgs...)
	return nil
}

// Read retrieves all stored messages for the specified conversation ID.
// Returns an empty slice if the conversation ID does not exist.
func (m *InMemoryMemory) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conversation, exists := m.store[conversationID]
	if !exists {
		return []chat.Message{}, nil
	}

	// Return a copy to prevent external modification
	result := make([]chat.Message, len(conversation))
	copy(result, conversation)
	return result, nil
}

// Clear removes all stored messages for the specified conversation ID.
func (m *InMemoryMemory) Clear(ctx context.Context, conversationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.store, conversationID)
	return nil
}
