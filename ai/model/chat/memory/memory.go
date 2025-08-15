package memory

import (
	"context"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

// Memory defines the contract for storing and managing the memory of chat conversations.
//
// Large language models (LLMs) are stateless, meaning they do not retain information about
// previous interactions. The Memory abstraction addresses this limitation by allowing
// you to store and retrieve information across multiple interactions with the LLM.
//
// Memory is designed to manage contextually relevant information that the LLM needs
// to maintain awareness throughout a conversation, not to store the complete chat history.
// Different implementations can use various strategies such as:
//   - Keeping the last N messages
//   - Keeping messages for a certain time period
//   - Keeping messages up to a certain token limit
//
// Note: Memory focuses on maintaining conversational context, while complete chat
// history storage should be handled by dedicated persistence solutions.
type Memory interface {
	// Add saves the specified messages in the chat memory for the specified conversation.
	// The implementation decides which messages to store and how to manage them based on
	// its memory strategy (e.g., filtering, merging, or discarding messages).
	Add(ctx context.Context, conversationID string, msgs ...messages.Message) error

	// Get retrieves the contextually relevant messages from the chat memory for the
	// specified conversation. The implementation decides which messages to return and
	// how to process them based on its memory strategy (e.g., applying sliding window,
	// token limits, or message prioritization). This represents the information that
	// should be provided to the LLM to maintain conversational context.
	Get(ctx context.Context, conversationID string) ([]messages.Message, error)

	// Clear removes all messages from the chat memory for the specified conversation.
	// This resets the conversational context for the given conversation.
	Clear(ctx context.Context, conversationID string) error
}

// Repository defines a repository for storing and retrieving chat messages.
//
// The MemoryRepository's sole responsibility is to store and retrieve messages.
// It serves as the underlying storage mechanism for Memory implementations,
// handling the actual persistence operations without any business logic about
// which messages to keep or remove.
//
// The decision of which messages to store, retrieve, or remove is delegated to
// the Memory implementation, allowing for flexible memory management strategies.
type Repository interface {
	// Find retrieves all stored messages for the given conversation ID from the
	// underlying storage mechanism.
	Find(ctx context.Context, conversationID string) ([]messages.Message, error)

	// Save persists the specified messages for the given conversation ID.
	// The repository stores the messages without applying any filtering logic.
	Save(ctx context.Context, conversationID string, msgs ...messages.Message) error

	// Delete removes all stored messages for the given conversation ID from the
	// underlying storage mechanism.
	Delete(ctx context.Context, conversationID string) error
}

const (
	// DefaultConversationID is the default conversation identifier used when
	// no specific conversation ID is provided.
	DefaultConversationID = "default"

	// ConversationIDKey is the key used to retrieve the chat memory conversation ID
	// from the context. This allows passing conversation identifiers through
	// the request context.
	ConversationIDKey = "conversation_id"
)
