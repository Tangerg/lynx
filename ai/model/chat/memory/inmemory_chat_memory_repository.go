package memory

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

var _ ChatMemoryRepository = (*InMemoryChatMemoryRepository)(nil)

// InMemoryChatMemoryRepository is an in-memory implementation of ChatMemoryRepository.
// It stores chat messages in memory using a map structure with read-write mutex
// for thread-safe concurrent access.
//
// Note: This implementation is suitable for development, testing, or scenarios
// where persistence across application restarts is not required. For production
// use cases requiring durability, consider using a persistent storage solution.
type InMemoryChatMemoryRepository struct {
	mu    sync.RWMutex
	store map[string][]messages.Message
}

// Find retrieves all stored messages for the given conversation ID.
// Returns an empty slice if no messages are found for the conversation ID.
// This method is thread-safe for concurrent read operations.
func (i *InMemoryChatMemoryRepository) Find(_ context.Context, conversationID string) ([]messages.Message, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	conversation, ok := i.store[conversationID]
	if !ok {
		return []messages.Message{}, nil
	}
	return conversation, nil
}

// Save appends the specified messages to the existing messages for the given conversation ID.
// If no messages exist for the conversation ID, a new conversation is created.
// Returns early if no messages are provided to save.
// This method is thread-safe for concurrent write operations.
func (i *InMemoryChatMemoryRepository) Save(_ context.Context, conversationID string, msgs ...messages.Message) error {
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

// Delete removes all messages for the given conversation ID from the repository.
// If the conversation ID doesn't exist, this operation is a no-op.
// This method is thread-safe for concurrent write operations.
func (i *InMemoryChatMemoryRepository) Delete(_ context.Context, conversationID string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.store, conversationID)
	return nil
}

// NewInMemoryChatMemoryRepository creates a new instance of InMemoryChatMemoryRepository
// with an initialized internal storage map.
func NewInMemoryChatMemoryRepository() *InMemoryChatMemoryRepository {
	return &InMemoryChatMemoryRepository{
		store: make(map[string][]messages.Message),
	}
}
