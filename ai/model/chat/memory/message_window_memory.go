package memory

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// MessageWindowMemory implements Memory with a sliding window strategy.
// It maintains a fixed number of recent messages while preserving system messages.
type MessageWindowMemory struct {
	maxMessages int
	repository  Repository
}

// Add stores messages for the conversation.
func (m *MessageWindowMemory) Add(ctx context.Context, conversationID string, msgs ...messages.Message) error {
	return m.repository.Save(ctx, conversationID, msgs...)
}

// Get retrieves and processes stored messages according to sliding window strategy.
func (m *MessageWindowMemory) Get(ctx context.Context, conversationID string) ([]messages.Message, error) {
	allMessages, err := m.repository.Find(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	return m.process(allMessages), nil
}

// process applies sliding window logic to keep messages within limit.
// System messages are merged and preserved, recent non-system messages are retained.
func (m *MessageWindowMemory) process(allMessages []messages.Message) []messages.Message {
	if len(allMessages) <= m.maxMessages {
		return allMessages
	}

	result := make([]messages.Message, 0, m.maxMessages)

	// Merge and preserve system messages
	systemMessage := messages.MergeSystemMessages(allMessages)
	if systemMessage != nil {
		result = append(result, systemMessage)
	}

	// Filter non-system messages
	nonSystemMessages := messages.FilterByTypes(allMessages, messages.User, messages.Assistant, messages.Tool)

	// Add recent non-system messages within remaining capacity
	remainingCapacity := m.maxMessages - len(result)
	if remainingCapacity > 0 && len(nonSystemMessages) > 0 {
		takeCount := min(remainingCapacity, len(nonSystemMessages))
		startIdx := len(nonSystemMessages) - takeCount
		result = append(result, nonSystemMessages[startIdx:]...)
	}

	return result
}

// Clear removes all messages for the conversation.
func (m *MessageWindowMemory) Clear(ctx context.Context, conversationID string) error {
	return m.repository.Delete(ctx, conversationID)
}

// NewMessageWindowMemory creates a new MessageWindowMemory instance.
// Limit is clamped to range [10, 100], defaults to 10 if not provided.
func NewMessageWindowMemory(repo Repository, limit ...int) (*MessageWindowMemory, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}

	maxMessages, _ := pkgSlices.First(limit)
	maxMessages = max(maxMessages, 10)
	maxMessages = min(maxMessages, 100)

	return &MessageWindowMemory{
		maxMessages: maxMessages,
		repository:  repo,
	}, nil
}
