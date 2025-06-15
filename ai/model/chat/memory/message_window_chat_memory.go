package memory

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// MessageWindowChatMemory implements ChatMemory interface with a sliding window strategy.
// It maintains a fixed number of recent messages to keep the conversation context
// within manageable limits while preserving system messages.
//
// This implementation uses a smart message retention strategy:
//   - System messages are always preserved and merged into a single message
//   - Non-system messages follow a LIFO (Last In, First Out) approach
//   - The total number of messages never exceeds the configured maximum
type MessageWindowChatMemory struct {
	// maxMessages defines the maximum number of messages to retain in memory.
	// When the limit is exceeded, older non-system messages are discarded.
	maxMessages int

	// chatMemoryRepository provides the underlying storage mechanism for persisting
	// and retrieving messages. It handles the actual data persistence operations.
	chatMemoryRepository ChatMemoryRepository
}

// Add stores the provided messages for the specified conversation by delegating
// to the underlying repository. All provided messages are saved without filtering.
//
// Parameters:
//   - ctx: The context for the operation, may contain cancellation signals
//   - conversationID: Unique identifier for the conversation
//   - msgs: Variable number of messages to be stored
//
// Returns:
//   - error: Any error that occurred during the storage operation
func (m *MessageWindowChatMemory) Add(ctx context.Context, conversationID string, msgs ...messages.Message) error {
	return m.chatMemoryRepository.Save(ctx, conversationID, msgs...)
}

// Get retrieves and processes the stored messages for the specified conversation.
// The returned messages are filtered and organized according to the sliding window strategy.
//
// Parameters:
//   - ctx: The context for the operation, may contain cancellation signals
//   - conversationID: Unique identifier for the conversation
//
// Returns:
//   - []messages.Message: Processed messages ready for LLM consumption
//   - error: Any error that occurred during retrieval or processing
func (m *MessageWindowChatMemory) Get(ctx context.Context, conversationID string) ([]messages.Message, error) {
	// Retrieve all stored messages from the repository
	msgs, err := m.chatMemoryRepository.Find(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	// Apply the sliding window processing strategy
	return m.process(msgs), nil
}

// process implements the core sliding window logic for message retention.
// It ensures the returned messages don't exceed the maximum limit while
// intelligently preserving important context.
//
// Processing strategy:
//  1. If total messages ≤ maxMessages, return all messages as-is
//  2. Separate system messages from other messages
//  3. Merge all system messages into a single consolidated message
//  4. Take the most recent non-system messages to fill remaining capacity
//
// Parameters:
//   - msgs: All messages retrieved from storage
//
// Returns:
//   - []messages.Message: Processed messages within the size limit
func (m *MessageWindowChatMemory) process(msgs []messages.Message) []messages.Message {
	// If we're within the limit, return
	if len(msgs) <= m.maxMessages {
		return msgs
	}

	processedMsgs := make([]messages.Message, 0, m.maxMessages)

	systemMsg := messages.MergeSystemMessages(msgs)
	if systemMsg != nil {
		processedMsgs = append(processedMsgs, systemMsg)
	}
	otherMsgs := messages.FilterByTypes(msgs, messages.User, messages.Assistant, messages.Tool)

	// Calculate remaining capacity for non-system messages
	remainingCap := m.maxMessages - len(processedMsgs)
	if remainingCap > 0 && len(otherMsgs) > 0 {
		// Take the most recent messages that fit in the remaining capacity
		takeCount := min(remainingCap, len(otherMsgs))
		startIndex := len(otherMsgs) - takeCount
		processedMsgs = append(processedMsgs, otherMsgs[startIndex:]...)
	}

	return processedMsgs
}

// Clear removes all stored messages for the specified conversation by
// delegating the operation to the underlying repository.
//
// Parameters:
//   - ctx: The context for the operation, may contain cancellation signals
//   - conversationID: Unique identifier for the conversation to clear
//
// Returns:
//   - error: Any error that occurred during the deletion operation
func (m *MessageWindowChatMemory) Clear(ctx context.Context, conversationID string) error {
	return m.chatMemoryRepository.Delete(ctx, conversationID)
}

// NewMessageWindowChatMemory creates a new instance of MessageWindowChatMemory
// with the specified configuration and validates the input parameters.
//
// Parameters:
//   - chatMemoryRepository: The repository for message persistence, must not be nil
//   - limit: Optional maximum number of messages to retain.
//     Valid range: 10-100. Values outside this range will be automatically
//     clamped to the nearest valid value. If not provided, defaults to 10.
//
// Returns:
//   - *MessageWindowChatMemory: Configured instance ready for use
//   - error: Validation error if repository is nil
//
// Examples:
//
//	// Using default limit (10)
//	memory, err := NewMessageWindowChatMemory(myRepository)
//
//	// Using custom limit (clamped to valid range)
//	memory, err := NewMessageWindowChatMemory(myRepository, 50)  // Uses 50
//	memory, err := NewMessageWindowChatMemory(myRepository, 5)   // Clamped to 10
//	memory, err := NewMessageWindowChatMemory(myRepository, 200) // Clamped to 100
func NewMessageWindowChatMemory(chatMemoryRepository ChatMemoryRepository, limit ...int) (*MessageWindowChatMemory, error) {
	if chatMemoryRepository == nil {
		return nil, errors.New("chat memory repository is required")
	}

	maxMessages, _ := pkgSlices.First(limit)
	maxMessages = max(maxMessages, 10)
	maxMessages = min(maxMessages, 100)

	return &MessageWindowChatMemory{
		maxMessages:          maxMessages,
		chatMemoryRepository: chatMemoryRepository,
	}, nil
}
