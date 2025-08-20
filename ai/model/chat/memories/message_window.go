package memories

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// MessageWindowMemory implements chat.Memory with a sliding window strategy.
// It maintains a fixed number of recent messages while preserving system messages
// by merging them and keeping the most recent non-system messages within the limit.
type MessageWindowMemory struct {
	maxMessages int
	inner       chat.Memory
}

// NewMessageWindowMemory creates a new MessageWindowMemory instance.
// The limit parameter specifies the maximum number of messages to retain.
// If not provided, defaults to 10. The limit is automatically clamped to [10, 100].
// Returns an error if the inner memory implementation is nil.
func NewMessageWindowMemory(inner chat.Memory, limit ...int) (*MessageWindowMemory, error) {
	if inner == nil {
		return nil, errors.New("inner memory implementation is required")
	}

	// Avoid double wrapping
	if existing, ok := inner.(*MessageWindowMemory); ok {
		return existing, nil
	}

	maxMessages := pkgSlices.AtOr(limit, 0, 0)

	// Clamp to valid range
	maxMessages = max(10, min(100, maxMessages))

	return &MessageWindowMemory{
		maxMessages: maxMessages,
		inner:       inner,
	}, nil
}

// Write stores messages for the specified conversation.
// The messages are delegated to the underlying memory implementation.
func (m *MessageWindowMemory) Write(ctx context.Context, conversationID string, msgs ...chat.Message) error {
	return m.inner.Write(ctx, conversationID, msgs...)
}

// Read retrieves and processes stored messages using the sliding window strategy.
// System messages are merged and preserved, while recent non-system messages
// are kept within the specified limit.
func (m *MessageWindowMemory) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	allMessages, err := m.inner.Read(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	return m.applyWindow(allMessages), nil
}

// applyWindow applies the sliding window strategy to the message list.
// It merges system messages and retains the most recent non-system messages
// within the configured limit.
func (m *MessageWindowMemory) applyWindow(allMessages []chat.Message) []chat.Message {
	if len(allMessages) <= m.maxMessages {
		return allMessages
	}

	result := make([]chat.Message, 0, m.maxMessages)

	// Merge and preserve system messages
	if systemMessage := messages.MergeSystemMessages(allMessages); systemMessage != nil {
		result = append(result, systemMessage)
	}

	// Filter and retain recent non-system messages
	nonSystemMessages := messages.FilterByTypes(allMessages, messages.User, messages.Assistant, messages.Tool)

	remainingCapacity := m.maxMessages - len(result)
	if remainingCapacity > 0 && len(nonSystemMessages) > 0 {
		startIdx := max(0, len(nonSystemMessages)-remainingCapacity)
		result = append(result, nonSystemMessages[startIdx:]...)
	}

	return result
}

// Clear removes all messages for the specified conversation.
// The operation is delegated to the underlying memory implementation.
func (m *MessageWindowMemory) Clear(ctx context.Context, conversationID string) error {
	return m.inner.Clear(ctx, conversationID)
}
