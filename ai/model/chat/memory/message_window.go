package memory

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ Store = (*MessageWindowStore)(nil)

// MessageWindowStore implements Store with a sliding window strategy.
// It maintains a fixed number of recent messages while preserving system messages
// by merging them and keeping the most recent non-system messages within the limit.
type MessageWindowStore struct {
	maximumMessages int
	store           Store
}

// NewMessageWindowMemory creates a new MessageWindowStore instance.
// The limit parameter specifies the maximum number of messages to retain.
// If not provided, defaults to 10. The limit is automatically clamped to [10, 100].
// Returns an error if the inner memory implementation is nil.
func NewMessageWindowMemory(storage Store, limit ...int) (*MessageWindowStore, error) {
	if storage == nil {
		return nil, errors.New("inner memory implementation cannot be nil")
	}

	// Avoid double wrapping
	if existing, ok := storage.(*MessageWindowStore); ok {
		return existing, nil
	}

	maxMsgCount := pkgSlices.AtOr(limit, 0, 10)

	// Clamp to valid range
	maxMsgCount = max(10, min(100, maxMsgCount))

	return &MessageWindowStore{
		maximumMessages: maxMsgCount,
		store:           storage,
	}, nil
}

// Write stores messages for the specified conversation.
// The messages are delegated to the underlying memory implementation.
func (m *MessageWindowStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	return m.store.Write(ctx, conversationID, messages...)
}

// Read retrieves and processes stored messages using the sliding window strategy.
// System messages are merged and preserved, while recent non-system messages
// are kept within the specified limit.
func (m *MessageWindowStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	all, err := m.store.Read(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	return m.applySlidingWindow(all), nil
}

// applySlidingWindow applies the sliding window strategy to the message list.
// It merges system messages and retains the most recent non-system messages
// within the configured limit.
func (m *MessageWindowStore) applySlidingWindow(all []chat.Message) []chat.Message {
	result := make([]chat.Message, 0, m.maximumMessages)

	// Merge and preserve system messages
	if sysMsg := chat.MergeSystemMessages(all); sysMsg != nil {
		result = append(result, sysMsg)
	}

	// Filter and retain recent non-system messages
	nonSys := chat.FilterMessagesByMessageTypes(all, chat.MessageTypeUser, chat.MessageTypeAssistant, chat.MessageTypeTool)

	remaining := m.maximumMessages - len(result)
	if remaining > 0 && len(nonSys) > 0 {
		start := max(0, len(nonSys)-remaining)
		result = append(result, nonSys[start:]...)
	}

	return result
}

// Clear removes all messages for the specified conversation.
// The operation is delegated to the underlying memory implementation.
func (m *MessageWindowStore) Clear(ctx context.Context, conversationID string) error {
	return m.store.Clear(ctx, conversationID)
}
