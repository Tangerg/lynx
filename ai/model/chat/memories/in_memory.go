package memories

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ chat.Memory = (*InMemoryMemory)(nil)

// InMemoryMemory is an in-memory implementation of Memory.
// It stores chat messages using a map with read-write mutex for thread safety.
// This implementation is suitable for development and testing environments
// but does not persist data across application restarts.
type InMemoryMemory struct {
	mutex                sync.RWMutex
	conversationMessages map[string][]chat.Message
}

func NewInMemoryMemory() *InMemoryMemory {
	return &InMemoryMemory{
		conversationMessages: make(map[string][]chat.Message),
	}
}

// Write stores the specified messages for the given conversation ID.
// If no messages are provided, the operation is a no-op.
func (m *InMemoryMemory) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.conversationMessages[conversationID] = append(m.conversationMessages[conversationID], messages...)
	return nil
}

// Read retrieves all stored messages for the specified conversation ID.
// Returns an empty slice if the conversation ID does not exist.
func (m *InMemoryMemory) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stored, exists := m.conversationMessages[conversationID]
	if !exists {
		return []chat.Message{}, nil
	}

	// Return a copy to prevent external modification
	copied := make([]chat.Message, len(stored))
	copy(copied, stored)
	return copied, nil
}

// Clear removes all stored messages for the specified conversation ID.
func (m *InMemoryMemory) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.conversationMessages, conversationID)
	return nil
}
