package chathistory

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

var (
	// ErrInvalidWindow reports a non-positive message limit.
	ErrInvalidWindow = errors.New("chathistory: invalid message window")
	// ErrListingUnsupported reports that a wrapped Store cannot enumerate
	// conversations.
	ErrListingUnsupported = errors.New("chathistory: conversation listing unsupported")
)

var (
	_ Store    = (*WindowStore)(nil)
	_ Lister   = (*WindowStore)(nil)
	_ Replacer = (*WindowStore)(nil)
)

// WindowStore projects reads to at most limit messages while preserving a
// merged system message followed by the most recent non-system messages.
// Writes and clears pass through to the authoritative Store.
type WindowStore struct {
	store Store
	limit int
}

// NewWindowStore returns a read-side sliding-window decorator. Limit counts
// the merged system message when one exists and must be greater than zero.
func NewWindowStore(store Store, limit int) (*WindowStore, error) {
	if store == nil {
		return nil, ErrNilStore
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: limit must be greater than zero", ErrInvalidWindow)
	}
	return &WindowStore{store: store, limit: limit}, nil
}

// Write delegates to the underlying Store.
func (s *WindowStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	return s.store.Write(ctx, conversationID, messages...)
}

// Read returns the windowed projection.
func (s *WindowStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	messages, err := s.store.Read(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return []chat.Message{}, nil
	}

	systems := make([]chat.Message, 0)
	nonSystem := make([]chat.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == chat.RoleSystem {
			systems = append(systems, message)
		} else {
			nonSystem = append(nonSystem, message)
		}
	}

	window := make([]chat.Message, 0, s.limit)
	if len(systems) > 0 {
		window = append(window, mergeSystemMessages(systems))
	}
	remaining := s.limit - len(window)
	if remaining > 0 && len(nonSystem) > 0 {
		start := max(0, len(nonSystem)-remaining)
		window = append(window, nonSystem[start:]...)
	}
	return window, nil
}

// Clear delegates to the underlying Store.
func (s *WindowStore) Clear(ctx context.Context, conversationID string) error {
	return s.store.Clear(ctx, conversationID)
}

// Replace delegates through the optional atomic capability helper.
func (s *WindowStore) Replace(ctx context.Context, conversationID string, messages ...chat.Message) error {
	return Replace(ctx, s.store, conversationID, messages...)
}

// Conversations delegates when the underlying Store implements Lister.
func (s *WindowStore) Conversations(ctx context.Context) ([]string, error) {
	lister, ok := s.store.(Lister)
	if !ok {
		return nil, ErrListingUnsupported
	}
	return lister.Conversations(ctx)
}

func mergeSystemMessages(messages []chat.Message) chat.Message {
	merged := chat.Message{Role: chat.RoleSystem, Metadata: metadata.Map{}}
	for i, message := range messages {
		if i > 0 {
			merged.Parts = append(merged.Parts, chat.NewTextPart("\n\n"))
		}
		merged.Parts = append(merged.Parts, chat.NewTextPart(message.Text()))
		for key, value := range message.Metadata {
			merged.Metadata[key] = append([]byte(nil), value...)
		}
	}
	if len(merged.Metadata) == 0 {
		merged.Metadata = nil
	}
	return merged
}
