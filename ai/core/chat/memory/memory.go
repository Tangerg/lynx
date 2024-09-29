package memory

import (
	"context"

	"github.com/Tangerg/lynx/core/message"
)

// Memory interface defines methods for managing conversation memory, allowing storage, retrieval, and clearing of messages.
type Memory interface {
	// Add stores one or more messages associated with a specific conversation ID.
	// Parameters:
	// - conversationId: A string representing the unique identifier for the conversation.
	// - messages: A variadic parameter of message.Message type representing the messages to be added.
	Add(ctx context.Context, conversationId string, messages ...message.Message) error

	// Get retrieves the last N messages from a specific conversation.
	// Parameters:
	// - conversationId: A string representing the unique identifier for the conversation.
	// - lastN: An integer specifying the number of recent messages to retrieve.
	// Returns:
	// - A slice of message.Message containing the retrieved messages.
	Get(ctx context.Context, conversationId string, lastN int) ([]message.Message, error)

	// Clear removes all messages associated with a specific conversation ID.
	// Parameters:
	// - conversationId: A string representing the unique identifier for the conversation to be cleared.
	Clear(ctx context.Context, conversationId string) error
}
