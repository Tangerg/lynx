package result

import (
	"iter"
	"maps"

	"github.com/Tangerg/lynx/ai/model/model"
)

// ChatResultMetadata represents the metadata associated with the generation of a chat response.
// It embeds the base ResultMetadata interface and provides additional functionality
// for accessing finish reasons and custom metadata fields.
//
// ChatResultMetadata instances are immutable once created and should be constructed using ChatResultMetadataBuilder.
type ChatResultMetadata struct {
	model.ResultMetadata
	fields       map[string]any
	finishReason FinishReason
}

// Get retrieves a custom metadata value by key.
// Returns the value and a boolean indicating whether the key exists.
// This method is safe for concurrent access as the underlying data is immutable.
func (m *ChatResultMetadata) Get(key string) (any, bool) {
	val, ok := m.fields[key]
	return val, ok
}

// FinishReason returns the reason why the generation process finished.
// This method is safe for concurrent access as the underlying data is immutable.
func (m *ChatResultMetadata) FinishReason() FinishReason {
	return m.finishReason
}

// Iter returns an iterator over all key-value pairs in the metadata fields.
// The iterator is safe for concurrent access as the underlying data is immutable.
//
// Example usage:
//
//	for key, value := range metadata.Iter() {
//	    fmt.Printf("%s: %v\n", key, value)
//	}
func (m *ChatResultMetadata) Iter() iter.Seq2[string, any] {
	return maps.All(m.fields)
}

// Size returns the number of custom metadata fields
func (m *ChatResultMetadata) Size() int {
	return len(m.fields)
}

// NewMetadata creates a new ChatResultMetadata instance with the specified finish reason and optional metadata fields.
// If finishReason is invalid/empty, it defaults to Null.
// The metadata map is safely cloned to ensure immutability.
func NewMetadata(finishReason FinishReason, metadata ...map[string]any) *ChatResultMetadata {
	if finishReason.String() == "" {
		finishReason = Null
	}

	fields := make(map[string]any)
	if len(metadata) > 0 && metadata[0] != nil {
		fields = maps.Clone(metadata[0])
	}

	return &ChatResultMetadata{
		fields:       fields,
		finishReason: finishReason,
	}
}

// ChatResultMetadataBuilder provides a fluent interface for constructing immutable ChatResultMetadata instances.
// It follows the builder pattern to allow method chaining for easy configuration.
// The builder maintains a mutable state during construction, but produces immutable results.
type ChatResultMetadataBuilder struct {
	fields       map[string]any
	finishReason FinishReason
}

// WithFinishReason sets the finish reason for the metadata being built.
// Returns the builder instance for method chaining.
// This method modifies the builder's internal state before the final build.
func (b *ChatResultMetadataBuilder) WithFinishReason(f FinishReason) *ChatResultMetadataBuilder {
	b.finishReason = f
	return b
}

// WithParam adds a single key-value pair to the metadata fields.
// Returns the builder instance for method chaining.
// This method modifies the builder's internal state before the final build.
func (b *ChatResultMetadataBuilder) WithParam(key string, value any) *ChatResultMetadataBuilder {
	b.fields[key] = value
	return b
}

// WithParams adds multiple key-value pairs to the metadata fields.
// The provided map is copied into the metadata, preserving existing fields.
// Returns the builder instance for method chaining.
// This method modifies the builder's internal state before the final build.
func (b *ChatResultMetadataBuilder) WithParams(params map[string]any) *ChatResultMetadataBuilder {
	maps.Copy(b.fields, params)
	return b
}

// Build returns the constructed immutable ChatResultMetadata instance.
// Once built, the returned ChatResultMetadata cannot be modified and is safe for concurrent access.
//
// WARNING: After calling Build, this builder instance should not be reused as it may
// lead to shared mutable state. Create a new ChatResultMetadataBuilder for constructing additional ChatResultMetadata instances.
func (b *ChatResultMetadataBuilder) Build() *ChatResultMetadata {
	return NewMetadata(b.finishReason, b.fields)
}

// NewChatResultMetadataBuilder creates a new ChatResultMetadataBuilder instance with default metadata.
// Use this to start building an immutable ChatResultMetadata instance.
func NewChatResultMetadataBuilder() *ChatResultMetadataBuilder {
	return &ChatResultMetadataBuilder{
		finishReason: Null,
		fields:       make(map[string]any),
	}
}
