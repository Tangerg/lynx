package response

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/result"
	"github.com/Tangerg/lynx/ai/model/model"
)

var _ model.Response[*result.ChatResult, *ChatResponseMetadata] = (*ChatResponse)(nil)

// ChatResponse represents a response from a chat AI model containing results and metadata.
type ChatResponse struct {
	results  []*result.ChatResult
	metadata *ChatResponseMetadata
}

// NewChatResponse creates a new ChatResponse with the provided results and metadata.
// Returns an error if results or metadata are nil.
func NewChatResponse(results []*result.ChatResult, metadata *ChatResponseMetadata) (*ChatResponse, error) {
	if results == nil {
		return nil, errors.New("results is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}
	return &ChatResponse{
		results:  results,
		metadata: metadata,
	}, nil
}

// Result returns the first chat result if available, otherwise returns nil.
func (c *ChatResponse) Result() *result.ChatResult {
	if len(c.results) > 0 {
		return c.results[0]
	}
	return nil
}

// Results returns all chat results.
func (c *ChatResponse) Results() []*result.ChatResult {
	return c.results
}

// Metadata returns the response metadata.
func (c *ChatResponse) Metadata() *ChatResponseMetadata {
	return c.metadata
}

// ChatResponseBuilder provides a builder pattern for creating ChatResponse instances.
type ChatResponseBuilder struct {
	results  []*result.ChatResult
	metadata *ChatResponseMetadata
}

// NewChatResponseBuilder creates a new ChatResponseBuilder instance.
func NewChatResponseBuilder() *ChatResponseBuilder {
	return &ChatResponseBuilder{}
}

// WithResult adds a single chat result to the results list if result is not nil.
func (c *ChatResponseBuilder) WithResult(result *result.ChatResult) *ChatResponseBuilder {
	if result != nil {
		c.results = append(c.results, result)
	}
	return c
}

// WithResults replaces all chat results with the provided results if results is not nil.
func (c *ChatResponseBuilder) WithResults(results []*result.ChatResult) *ChatResponseBuilder {
	if results != nil {
		c.results = results
	}
	return c
}

// WithMetadata sets the response metadata if metadata is not nil.
func (c *ChatResponseBuilder) WithMetadata(metadata *ChatResponseMetadata) *ChatResponseBuilder {
	if metadata != nil {
		c.metadata = metadata
	}
	return c
}

// Build creates a new ChatResponse instance with the configured values.
// Returns an error if results or metadata are not set.
func (c *ChatResponseBuilder) Build() (*ChatResponse, error) {
	return NewChatResponse(c.results, c.metadata)
}

// MustBuild creates a new ChatResponse instance with the configured values.
// Panics if results or metadata are not set.
func (c *ChatResponseBuilder) MustBuild() *ChatResponse {
	response, err := c.Build()
	if err != nil {
		panic(err)
	}
	return response
}
