package result

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/model"
)

var _ model.Result[*messages.AssistantMessage, *ChatResultMetadata] = (*ChatResult)(nil)

// ChatResult represents the result of a chat completion request.
// It contains the assistant's response message and associated metadata.
// ChatResult instances are immutable and safe for concurrent access.
type ChatResult struct {
	assistantMessage *messages.AssistantMessage // The assistant's response message
	metadata         *ChatResultMetadata        // Metadata associated with the chat completion
}

// Output returns the assistant's response message.
// This method is safe for concurrent access as the underlying data is immutable.
func (r *ChatResult) Output() *messages.AssistantMessage {
	return r.assistantMessage
}

// Metadata returns the metadata associated with the chat completion.
// This includes information such as the finish reason and custom metadata fields.
// This method is safe for concurrent access as the underlying data is immutable.
func (r *ChatResult) Metadata() *ChatResultMetadata {
	return r.metadata
}

// NewChatResult creates a new ChatResult with the given assistant message and metadata.
// Returns an error if either parameter is nil, ensuring ChatResult instances are always valid.
func NewChatResult(assistantMessage *messages.AssistantMessage, metadata *ChatResultMetadata) (*ChatResult, error) {
	if assistantMessage == nil {
		return nil, errors.New("assistant message is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}
	return &ChatResult{
		assistantMessage: assistantMessage,
		metadata:         metadata,
	}, nil
}

// ChatResultBuilder provides a fluent interface for constructing immutable ChatResult instances.
// It follows the builder pattern to allow method chaining for easy configuration.
// The builder maintains a mutable state during construction, but produces immutable results.
type ChatResultBuilder struct {
	assistantMessage *messages.AssistantMessage
	metadata         *ChatResultMetadata
}

// NewChatResultBuilder creates a new ChatResultBuilder instance.
// Use this to start building an immutable ChatResult instance with method chaining.
func NewChatResultBuilder() *ChatResultBuilder {
	return &ChatResultBuilder{}
}

// WithAssistantMessage sets the assistant message for the chat result being built if message is not nil.
// Returns the builder instance for method chaining.
func (b *ChatResultBuilder) WithAssistantMessage(message *messages.AssistantMessage) *ChatResultBuilder {
	if message != nil {
		b.assistantMessage = message
	}
	return b
}

// WithMetadata sets the metadata for the chat result being built if metadata is not nil.
// Returns the builder instance for method chaining.
func (b *ChatResultBuilder) WithMetadata(metadata *ChatResultMetadata) *ChatResultBuilder {
	if metadata != nil {
		b.metadata = metadata
	}
	return b
}

// Build constructs and returns the immutable ChatResult instance.
// Returns an error if required fields are missing, delegating validation to NewChatResult.
// This builder instance can be safely reused after calling Build.
func (b *ChatResultBuilder) Build() (*ChatResult, error) {
	return NewChatResult(b.assistantMessage, b.metadata)
}

// MustBuild constructs and returns the immutable ChatResult instance.
// Panics if validation fails, making it suitable for cases where you're confident
// about the validity of the inputs or want to fail fast during development/testing.
//
// Use Build() if you prefer explicit error handling in production code.
func (b *ChatResultBuilder) MustBuild() *ChatResult {
	result, err := b.Build()
	if err != nil {
		panic(err)
	}
	return result
}
