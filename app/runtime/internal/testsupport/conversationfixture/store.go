// Package conversationfixture provides app-port-compatible conversation
// storage for tests without coupling Application production code to the
// reusable history contract.
package conversationfixture

import (
	"context"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
	"github.com/Tangerg/scope/core/history/inmemory"
)

// Store adapts the reusable in-memory history store to Runtime's session-ID
// conversation ports. The adapter is intentionally test-only: production uses
// the Runtime-owned SQLite MessageStore directly.
type Store struct {
	backend *inmemory.Store
}

// New returns an empty app-port-compatible conversation store.
func New() *Store {
	return &Store{backend: inmemory.New()}
}

// Read returns the messages stored for sessionID.
func (s *Store) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return s.backend.Read(ctx, history.ConversationID(sessionID))
}

// Write appends messages to sessionID.
func (s *Store) Write(ctx context.Context, sessionID string, messages ...chat.Message) error {
	return s.backend.Write(ctx, history.ConversationID(sessionID), messages...)
}

// Clear removes sessionID's messages.
func (s *Store) Clear(ctx context.Context, sessionID string) error {
	return s.backend.Clear(ctx, history.ConversationID(sessionID))
}

// Replace atomically sets sessionID's messages.
func (s *Store) Replace(ctx context.Context, sessionID string, messages ...chat.Message) error {
	return s.backend.Replace(ctx, history.ConversationID(sessionID), messages...)
}

// Count returns sessionID's message count.
func (s *Store) Count(ctx context.Context, sessionID string) (int, error) {
	return s.backend.Count(ctx, history.ConversationID(sessionID))
}
