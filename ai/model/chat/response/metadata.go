package response

import (
	"iter"
	"maps"

	"github.com/Tangerg/lynx/ai/model/model"
)

// ChatResponseMetadata encapsulates metadata returned in chat AI responses.
type ChatResponseMetadata struct {
	model.ResponseMetadata
	id        string
	model     string
	usage     *Usage
	rateLimit *RateLimit
	fields    map[string]any
}

// Get retrieves a custom field value by key.
func (c *ChatResponseMetadata) Get(key string) (any, bool) {
	val, ok := c.fields[key]
	return val, ok
}

// ID returns the unique identifier of the response.
func (c *ChatResponseMetadata) ID() string {
	return c.id
}

// Model returns the model name used for the response.
func (c *ChatResponseMetadata) Model() string {
	return c.model
}

// Usage returns the token usage information.
func (c *ChatResponseMetadata) Usage() *Usage {
	return c.usage
}

// RateLimit returns the rate limit information.
func (c *ChatResponseMetadata) RateLimit() *RateLimit {
	return c.rateLimit
}

// Iter returns an iterator over all custom fields.
func (c *ChatResponseMetadata) Iter() iter.Seq2[string, any] {
	return maps.All(c.fields)
}

// Size returns the number of custom fields.
func (c *ChatResponseMetadata) Size() int {
	return len(c.fields)
}

// ChatResponseMetadataBuilder provides a builder pattern for creating ChatResponseMetadata instances.
type ChatResponseMetadataBuilder struct {
	id        string
	model     string
	usage     *Usage
	rateLimit *RateLimit
	fields    map[string]any
}

// NewChatResponseMetadataBuilder creates a new ChatResponseMetadataBuilder instance.
func NewChatResponseMetadataBuilder() *ChatResponseMetadataBuilder {
	return &ChatResponseMetadataBuilder{
		usage:     NewUsageBuilder().Build(),
		rateLimit: NewRateLimitBuilder().Build(),
		fields:    make(map[string]any),
	}
}

// WithID sets the response ID if it's not empty.
func (c *ChatResponseMetadataBuilder) WithID(id string) *ChatResponseMetadataBuilder {
	if id != "" {
		c.id = id
	}
	return c
}

// WithModel sets the model name if it's not empty.
func (c *ChatResponseMetadataBuilder) WithModel(model string) *ChatResponseMetadataBuilder {
	if model != "" {
		c.model = model
	}
	return c
}

// WithUsage sets the usage information if it's not nil.
func (c *ChatResponseMetadataBuilder) WithUsage(usage *Usage) *ChatResponseMetadataBuilder {
	if usage != nil {
		c.usage = usage
	}
	return c
}

// WithRateLimit sets the rate limit information if it's not nil.
func (c *ChatResponseMetadataBuilder) WithRateLimit(rateLimit *RateLimit) *ChatResponseMetadataBuilder {
	if rateLimit != nil {
		c.rateLimit = rateLimit
	}
	return c
}

// WithField adds a custom field if key is not empty
func (c *ChatResponseMetadataBuilder) WithField(key string, value any) *ChatResponseMetadataBuilder {
	if key != "" {
		c.fields[key] = value
	}
	return c
}

// WithFields adds multiple custom fields, filtering out empty keys
func (c *ChatResponseMetadataBuilder) WithFields(fields map[string]any) *ChatResponseMetadataBuilder {
	if fields != nil {
		for k, v := range fields {
			c.WithField(k, v)
		}
	}
	return c
}

// Build creates a new ChatResponseMetadata instance with the configured values.
func (c *ChatResponseMetadataBuilder) Build() *ChatResponseMetadata {
	return &ChatResponseMetadata{
		id:        c.id,
		model:     c.model,
		usage:     c.usage,
		rateLimit: c.rateLimit,
		fields:    c.fields,
	}
}
