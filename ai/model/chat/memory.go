package chat

import (
	"context"
	"errors"
	"sync"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// MemoryReader defines the interface for reading conversational context from memory.
type MemoryReader interface {
	// Read retrieves contextually relevant messages for the specified conversation.
	// The implementation determines which messages to return based on its memory
	// strategy (e.g., sliding window, token limits, or message prioritization).
	// The returned messages represent the context that should be provided to
	// the LLM to maintain conversational continuity.
	Read(ctx context.Context, conversationID string) ([]Message, error)
}

// MemoryWriter defines the interface for writing conversational context to memory.
type MemoryWriter interface {
	// Write stores the specified messages in memory for the given conversation.
	// The implementation determines which messages to retain and how to manage
	// them based on its memory strategy (e.g., filtering, merging, or evicting
	// older messages).
	Write(ctx context.Context, conversationID string, messages ...Message) error
}

// MemoryClearer defines the interface for clearing conversational context from memory.
type MemoryClearer interface {
	// Clear removes all stored messages for the specified conversation,
	// effectively resetting the conversational context.
	Clear(ctx context.Context, conversationID string) error
}

// Memory defines the interface for storing and managing conversational context
// across chat interactions.
//
// Large language models (LLMs) are stateless and cannot retain information from
// previous interactions. The Memory interface addresses this limitation by enabling
// storage and retrieval of contextual information across multiple LLM interactions.
//
// Memory is designed to manage contextually relevant information that helps the LLM
// maintain conversational awareness, rather than storing complete chat history.
// Different implementations can employ various retention strategies:
//   - Retain the last N messages
//   - Retain messages within a specific time window
//   - Retain messages within token count limits
//   - Apply message prioritization or summarization
//
// Note: Memory focuses on maintaining conversational context for LLM interactions.
// Complete chat history persistence should be handled by dedicated storage solutions.
type Memory interface {
	MemoryReader
	MemoryWriter
	MemoryClearer
}

// InMemoryMemory is an in-memory implementation of Memory.
// It stores chat messages using a map with read-write mutex for thread safety.
// This implementation is suitable for development and testing environments
// but does not persist data across application restarts.
type InMemoryMemory struct {
	mutex                sync.RWMutex
	conversationMessages map[string][]Message
}

func NewInMemoryMemory() *InMemoryMemory {
	return &InMemoryMemory{
		conversationMessages: make(map[string][]Message),
	}
}

// Write stores the specified messages for the given conversation ID.
// If no messages are provided, the operation is a no-op.
func (m *InMemoryMemory) Write(ctx context.Context, conversationID string, messages ...Message) error {
	if len(messages) == 0 {
		return nil
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.conversationMessages[conversationID] = append(m.conversationMessages[conversationID], messages...)
	return nil
}

// Read retrieves all stored messages for the specified conversation ID.
// Returns an empty slice if the conversation ID does not exist.
func (m *InMemoryMemory) Read(ctx context.Context, conversationID string) ([]Message, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stored, exists := m.conversationMessages[conversationID]
	if !exists {
		return []Message{}, nil
	}

	// Return a copy to prevent external modification
	copied := make([]Message, len(stored))
	copy(copied, stored)
	return copied, nil
}

// Clear removes all stored messages for the specified conversation ID.
func (m *InMemoryMemory) Clear(ctx context.Context, conversationID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.conversationMessages, conversationID)
	return nil
}

// MessageWindowMemory implements Memory with a sliding window strategy.
// It maintains a fixed number of recent messages while preserving system messages
// by merging them and keeping the most recent non-system messages within the limit.
type MessageWindowMemory struct {
	maximumMessages int
	innerMemory     Memory
}

// NewMessageWindowMemory creates a new MessageWindowMemory instance.
// The limit parameter specifies the maximum number of messages to retain.
// If not provided, defaults to 10. The limit is automatically clamped to [10, 100].
// Returns an error if the inner memory implementation is nil.
func NewMessageWindowMemory(innerMemory Memory, limit ...int) (*MessageWindowMemory, error) {
	if innerMemory == nil {
		return nil, errors.New("inner memory implementation cannot be nil")
	}

	// Avoid double wrapping
	if existing, ok := innerMemory.(*MessageWindowMemory); ok {
		return existing, nil
	}

	maxMsgCount := pkgSlices.AtOr(limit, 0, 10)

	// Clamp to valid range
	maxMsgCount = max(10, min(100, maxMsgCount))

	return &MessageWindowMemory{
		maximumMessages: maxMsgCount,
		innerMemory:     innerMemory,
	}, nil
}

// Write stores messages for the specified conversation.
// The messages are delegated to the underlying memory implementation.
func (m *MessageWindowMemory) Write(ctx context.Context, conversationID string, messages ...Message) error {
	return m.innerMemory.Write(ctx, conversationID, messages...)
}

// Read retrieves and processes stored messages using the sliding window strategy.
// System messages are merged and preserved, while recent non-system messages
// are kept within the specified limit.
func (m *MessageWindowMemory) Read(ctx context.Context, conversationID string) ([]Message, error) {
	all, err := m.innerMemory.Read(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	return m.applySlidingWindow(all), nil
}

// applySlidingWindow applies the sliding window strategy to the message list.
// It merges system messages and retains the most recent non-system messages
// within the configured limit.
func (m *MessageWindowMemory) applySlidingWindow(all []Message) []Message {
	if len(all) <= m.maximumMessages {
		return all
	}

	result := make([]Message, 0, m.maximumMessages)

	// Merge and preserve system messages
	if sysMsg := MergeSystemMessages(all); sysMsg != nil {
		result = append(result, sysMsg)
	}

	// Filter and retain recent non-system messages
	nonSys := FilterMessagesByMessageTypes(all, MessageTypeUser, MessageTypeAssistant, MessageTypeTool)

	remaining := m.maximumMessages - len(result)
	if remaining > 0 && len(nonSys) > 0 {
		start := max(0, len(nonSys)-remaining)
		result = append(result, nonSys[start:]...)
	}

	return result
}

// Clear removes all messages for the specified conversation.
// The operation is delegated to the underlying memory implementation.
func (m *MessageWindowMemory) Clear(ctx context.Context, conversationID string) error {
	return m.innerMemory.Clear(ctx, conversationID)
}
