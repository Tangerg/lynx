package result

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/model"
)

var _ model.Result[*messages.AssistantMessage, *ChatResultMetadata] = (*ChatResult)(nil)

// ChatResult represents the result of a chat completion request.
// It contains the assistant's response message and associated metadata.
//
// ChatResult supports both standard AI responses and tool-enhanced conversations:
// - Standard chat: Contains AI model's direct response with metadata
// - Tool-enhanced chat: May include tool calls and tool execution responses
type ChatResult struct {
	assistantMessage    *messages.AssistantMessage    // The assistant's response message (required)
	metadata            *ChatResultMetadata           // Metadata associated with the chat completion (required)
	toolResponseMessage *messages.ToolResponseMessage // Optional tool response for completed tool executions
}

// ToolResponseMessage returns the tool response message if present.
// Returns nil if no tool response is available.
func (r *ChatResult) ToolResponseMessage() *messages.ToolResponseMessage {
	return r.toolResponseMessage
}

// Output returns the assistant's response message.
// This contains the AI model's direct response, which may include tool calls.
func (r *ChatResult) Output() *messages.AssistantMessage {
	return r.assistantMessage
}

// Metadata returns the metadata associated with the chat completion.
// This includes information such as finish reason, token usage, and model details.
func (r *ChatResult) Metadata() *ChatResultMetadata {
	return r.metadata
}

// NewChatResult creates a new ChatResult with the given assistant message and metadata.
// Both parameters are required to ensure ChatResult instances are always in a valid state.
//
// Parameters:
//   - assistantMessage: The AI model's response message (required)
//   - metadata: Metadata associated with the chat completion (required)
//
// Returns an error if either required parameter is nil.
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

// ChatResultBuilder provides a fluent interface for constructing ChatResult instances.
// It follows the builder pattern to allow method chaining and supports optional components
// like tool response messages.
//
// Usage examples:
//   - Simple chat: WithAssistantMessage().WithMetadata().Build()
//   - Tool-enhanced: WithAssistantMessage().WithMetadata().WithToolResponseMessage().Build()
type ChatResultBuilder struct {
	assistantMessage    *messages.AssistantMessage
	metadata            *ChatResultMetadata
	toolResponseMessage *messages.ToolResponseMessage
}

// NewChatResultBuilder creates a new ChatResultBuilder instance.
// Use this to start building a ChatResult instance with method chaining.
func NewChatResultBuilder() *ChatResultBuilder {
	return &ChatResultBuilder{}
}

// WithAssistantMessage sets the assistant message for the chat result being built.
// The assistant message is required for all ChatResult instances.
//
// This method safely ignores nil values, allowing for conditional configuration
// without breaking the method chain.
func (b *ChatResultBuilder) WithAssistantMessage(message *messages.AssistantMessage) *ChatResultBuilder {
	if message != nil {
		b.assistantMessage = message
	}
	return b
}

// WithMetadata sets the metadata for the chat result being built.
// Metadata is required for all ChatResult instances and contains completion information
// such as finish reason and token usage.
//
// This method safely ignores nil values for method chaining compatibility.
func (b *ChatResultBuilder) WithMetadata(metadata *ChatResultMetadata) *ChatResultBuilder {
	if metadata != nil {
		b.metadata = metadata
	}
	return b
}

// WithToolResponseMessage sets the optional tool response message.
// This is used when tool execution has been completed and the results are available.
//
// This method safely ignores nil values for method chaining compatibility.
func (b *ChatResultBuilder) WithToolResponseMessage(toolResponse *messages.ToolResponseMessage) *ChatResultBuilder {
	if toolResponse != nil {
		b.toolResponseMessage = toolResponse
	}
	return b
}

// Build constructs and returns the ChatResult instance.
// This method validates that all required components (assistant message and metadata)
// are properly configured before creating the result.
//
// Returns an error if validation fails due to missing required components.
func (b *ChatResultBuilder) Build() (*ChatResult, error) {
	chatResult, err := NewChatResult(b.assistantMessage, b.metadata)
	if err != nil {
		return nil, err
	}
	chatResult.toolResponseMessage = b.toolResponseMessage
	return chatResult, nil
}

// MustBuild constructs and returns the ChatResult instance.
// Panics if validation fails, making it suitable for cases where you're confident
// about the validity of the inputs or want to fail fast during development.
//
// Use Build() if you prefer explicit error handling in production code.
func (b *ChatResultBuilder) MustBuild() *ChatResult {
	result, err := b.Build()
	if err != nil {
		panic(err)
	}
	return result
}
