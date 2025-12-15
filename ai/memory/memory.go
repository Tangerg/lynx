package memory

import (
	"context"

	"github.com/Tangerg/lynx/ai/model/chat"
)

// Reader defines the interface for reading conversational context from memory.
type Reader interface {
	// Read retrieves contextually relevant messages for the specified conversation.
	// The implementation determines which messages to return based on its memory
	// strategy (e.g., sliding window, token limits, or message prioritization).
	// The returned messages represent the context that should be provided to
	// the LLM to maintain conversational continuity.
	Read(ctx context.Context, conversationID string) ([]chat.Message, error)
}

// Writer defines the interface for writing conversational context to memory.
type Writer interface {
	// Write stores the specified messages in memory for the given conversation.
	// The implementation determines which messages to retain and how to manage
	// them based on its memory strategy (e.g., filtering, merging, or evicting
	// older messages).
	Write(ctx context.Context, conversationID string, messages ...chat.Message) error
}

// Clearer defines the interface for clearing conversational context from memory.
type Clearer interface {
	// Clear removes all stored messages for the specified conversation,
	// effectively resetting the conversational context.
	Clear(ctx context.Context, conversationID string) error
}

// Store defines the interface for storing and managing conversational context
// across chat interactions.
//
// Large language models (LLMs) are stateless and cannot retain information from
// previous interactions. The Store interface addresses this limitation by enabling
// store and retrieval of contextual information across multiple LLM interactions.
//
// Store is designed to manage contextually relevant information that helps the LLM
// maintain conversational awareness, rather than storing complete chat history.
// Different implementations can employ various retention strategies:
//   - Retain the last N messages
//   - Retain messages within a specific time window
//   - Retain messages within token count limits
//   - Apply message prioritization or summarization
//
// Note: Store focuses on maintaining conversational context for LLM interactions.
// Complete chat history persistence should be handled by dedicated store solutions.
type Store interface {
	Reader
	Writer
	Clearer
}
