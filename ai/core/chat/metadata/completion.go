package metadata

import (
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.ResponseMetadata = (*ChatCompletionMetadata)(nil)

type ChatCompletionMetadata struct {
	id       string
	model    string
	metadata map[string]any
	usage    Usage
	created  int64
}

func (c *ChatCompletionMetadata) Get(key string) (any, bool) {
	v, ok := c.metadata[key]
	return v, ok
}

func (c *ChatCompletionMetadata) GetOrDefault(key string, def any) any {
	v, ok := c.metadata[key]
	if !ok {
		v = def
	}
	return v
}

func (c *ChatCompletionMetadata) ID() string {
	return c.id
}

func (c *ChatCompletionMetadata) Model() string {
	return c.model
}

func (c *ChatCompletionMetadata) Usage() Usage {
	return c.usage
}

func (c *ChatCompletionMetadata) Created() int64 {
	return c.created
}

func newChatCompletionMetadata() *ChatCompletionMetadata {
	return &ChatCompletionMetadata{
		metadata: make(map[string]any),
		usage:    &EmptyUsage{},
	}
}

type ChatCompletionMetadataBuilder struct {
	metadata *ChatCompletionMetadata
}

func NewChatCompletionMetadataBuilder() *ChatCompletionMetadataBuilder {
	return &ChatCompletionMetadataBuilder{
		metadata: newChatCompletionMetadata(),
	}
}

func (b *ChatCompletionMetadataBuilder) WithID(id string) *ChatCompletionMetadataBuilder {
	b.metadata.id = id
	return b
}

func (b *ChatCompletionMetadataBuilder) WithModel(model string) *ChatCompletionMetadataBuilder {
	b.metadata.model = model
	return b
}

func (b *ChatCompletionMetadataBuilder) WithUsage(usage Usage) *ChatCompletionMetadataBuilder {
	b.metadata.usage = usage
	return b
}

func (b *ChatCompletionMetadataBuilder) WithCreated(created int64) *ChatCompletionMetadataBuilder {
	b.metadata.created = created
	return b
}

func (b *ChatCompletionMetadataBuilder) WithKeyValue(key string, value any) *ChatCompletionMetadataBuilder {
	b.metadata.metadata[key] = value
	return b
}

func (b *ChatCompletionMetadataBuilder) WithMetadata(metadata map[string]any) *ChatCompletionMetadataBuilder {
	for k, v := range metadata {
		b.metadata.metadata[k] = v
	}
	return b
}

func (b *ChatCompletionMetadataBuilder) Build() *ChatCompletionMetadata {
	return b.metadata
}
