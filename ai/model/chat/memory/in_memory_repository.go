package memory

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

var _ Repository = (*InMemoryRepository)(nil)

// InMemoryRepository is an in-memory implementation of Repository.
// It stores chat messages using a map with read-write mutex for thread safety.
// Suitable for development and testing; not persistent across restarts.
type InMemoryRepository struct {
	mu    sync.RWMutex
	store map[string][]messages.Message
}

// Find retrieves all messages for the given conversation ID.
// Returns empty slice if conversation not found.
func (i *InMemoryRepository) Find(_ context.Context, conversationID string) ([]messages.Message, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	conversation, ok := i.store[conversationID]
	if !ok {
		return []messages.Message{}, nil
	}
	return conversation, nil
}

// Save appends messages to the existing conversation.
// Creates new conversation if it doesn't exist.
func (i *InMemoryRepository) Save(_ context.Context, conversationID string, msgs ...messages.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	conversation := i.store[conversationID]
	conversation = append(conversation, msgs...)
	i.store[conversationID] = conversation
	return nil
}

// Delete removes all messages for the given conversation ID.
func (i *InMemoryRepository) Delete(_ context.Context, conversationID string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.store, conversationID)
	return nil
}

// NewMemoryRepository creates a new InMemoryRepository instance.
func NewMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		store: make(map[string][]messages.Message),
	}
}
