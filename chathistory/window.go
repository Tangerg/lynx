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
	// ErrWindowTooSmall reports that the newest complete conversation turn does
	// not fit within the configured message limit.
	ErrWindowTooSmall = errors.New("chathistory: message window too small")
)

var _ Store = (*WindowStore)(nil)

// WindowStore projects reads to at most limit messages while preserving a
// merged system message followed by a suffix of complete conversation turns.
// A user message starts a turn; every following assistant and tool message
// remains in that turn until the next user message. Writes and clears pass
// through to the authoritative Store.
type WindowStore struct {
	store Store
	limit int
}

// NewWindowStore returns a read-side sliding-window decorator. Limit counts
// the merged system message when one exists and must be greater than zero.
func NewWindowStore(store Store, limit int) (*WindowStore, error) {
	if isNilCapability(store) {
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

// Read returns the windowed projection. It returns ErrWindowTooSmall rather
// than splitting the newest turn when that turn does not fit.
func (s *WindowStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	messages, err := s.store.Read(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return newMessageWindow(messages, s.limit).project()
}

// Clear delegates to the underlying Store.
func (s *WindowStore) Clear(ctx context.Context, conversationID string) error {
	return s.store.Clear(ctx, conversationID)
}

type conversationTurn []chat.Message

type conversationTurns []conversationTurn

func (turns conversationTurns) suffix(limit int) ([]chat.Message, error) {
	if len(turns) == 0 {
		return []chat.Message{}, nil
	}
	newestTurnSize := len(turns[len(turns)-1])
	if newestTurnSize > limit {
		return nil, fmt.Errorf(
			"%w: newest turn has %d messages, available limit is %d",
			ErrWindowTooSmall,
			newestTurnSize,
			limit,
		)
	}

	start := len(turns) - 1
	messageCount := newestTurnSize
	for start > 0 {
		previousSize := len(turns[start-1])
		if previousSize > limit-messageCount {
			break
		}
		start--
		messageCount += previousSize
	}

	messages := make([]chat.Message, 0, messageCount)
	for _, turn := range turns[start:] {
		messages = append(messages, turn...)
	}
	return messages, nil
}

type messageWindow struct {
	limit          int
	systemMessages []chat.Message
	turns          conversationTurns
}

func newMessageWindow(messages []chat.Message, limit int) messageWindow {
	window := messageWindow{limit: limit}
	for _, message := range messages {
		if message.Role == chat.RoleSystem {
			window.systemMessages = append(window.systemMessages, message)
			continue
		}
		if len(window.turns) == 0 || message.Role == chat.RoleUser {
			window.turns = append(window.turns, conversationTurn{})
		}
		last := len(window.turns) - 1
		window.turns[last] = append(window.turns[last], message)
	}
	return window
}

func (window messageWindow) project() ([]chat.Message, error) {
	projected := make([]chat.Message, 0, window.limit)
	if len(window.systemMessages) > 0 {
		merged, err := window.mergedSystemMessage()
		if err != nil {
			return nil, err
		}
		projected = append(projected, merged)
	}

	recent, err := window.turns.suffix(window.limit - len(projected))
	if err != nil {
		return nil, err
	}
	return append(projected, recent...), nil
}

func (window messageWindow) mergedSystemMessage() (chat.Message, error) {
	merged := chat.Message{Role: chat.RoleSystem, Metadata: metadata.Map{}}
	for i, message := range window.systemMessages {
		if i > 0 {
			merged.Parts = append(merged.Parts, chat.NewTextPart("\n\n"))
		}
		for _, part := range message.Parts {
			merged.Parts = append(merged.Parts, part.Clone())
		}
		if err := merged.Metadata.Merge(message.Metadata); err != nil {
			return chat.Message{}, fmt.Errorf("chathistory: merge system message %d metadata: %w", i, err)
		}
	}
	if len(merged.Metadata) == 0 {
		merged.Metadata = nil
	}
	return merged, nil
}
