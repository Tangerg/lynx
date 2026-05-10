package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
)

var _ Store = (*InMemoryStore)(nil)

// InMemoryStore is an [Store] implementation backed by an in-process map
// guarded by an RWMutex. Suitable for development and single-instance
// services; data is lost on restart.
type InMemoryStore struct {
	mu    sync.RWMutex
	store map[string][]chat.Message
}

// NewInMemoryStore returns an empty [InMemoryStore].
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		store: make(map[string][]chat.Message),
	}
}

// Write appends messages under conversationID. No-op when messages is
// empty. Honors ctx cancellation.
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

// Read returns a defensive copy of the messages stored under
// conversationID. An empty slice is returned for unknown ids — never nil.
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
	return slices.Clone(stored), nil
}

// Clear drops every message stored under conversationID. Unknown ids
// are silently ignored.
func (m *InMemoryStore) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, conversationID)
	return nil
}
