package response

import (
	"github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/pkg/kv"
)

type ChatResponseMetadata struct {
	model.ResponseMetadata
	id        string
	model     string
	metadata  kv.KSVA
	usage     Usage
	rateLimit RateLimit
	created   int64
}

func (c *ChatResponseMetadata) ID() string {
	return c.id
}

func (c *ChatResponseMetadata) Model() string {
	return c.model
}

func (c *ChatResponseMetadata) Usage() Usage {
	return c.usage
}

func (c *ChatResponseMetadata) RateLimit() RateLimit {
	return c.rateLimit
}

func (c *ChatResponseMetadata) Created() int64 {
	return c.created
}

func (c *ChatResponseMetadata) Get(key string) (any, bool) {
	return c.metadata.Get(key)
}

func newChatResponseMetadata() *ChatResponseMetadata {
	return &ChatResponseMetadata{
		metadata:  kv.NewKSVA(),
		usage:     &EmptyUsage{},
		rateLimit: &EmptyRateLimit{},
	}
}

type ChatResponseMetadataBuilder struct {
	metadata *ChatResponseMetadata
}

func NewChatResponseMetadataBuilder() *ChatResponseMetadataBuilder {
	return &ChatResponseMetadataBuilder{
		metadata: newChatResponseMetadata(),
	}
}

func (b *ChatResponseMetadataBuilder) WithID(id string) *ChatResponseMetadataBuilder {
	b.metadata.id = id
	return b
}

func (b *ChatResponseMetadataBuilder) WithModel(model string) *ChatResponseMetadataBuilder {
	b.metadata.model = model
	return b
}

func (b *ChatResponseMetadataBuilder) WithUsage(usage Usage) *ChatResponseMetadataBuilder {
	b.metadata.usage = usage
	return b
}

func (b *ChatResponseMetadataBuilder) WithRateLimit(rateLimit RateLimit) *ChatResponseMetadataBuilder {
	b.metadata.rateLimit = rateLimit
	return b
}

func (b *ChatResponseMetadataBuilder) WithCreated(created int64) *ChatResponseMetadataBuilder {
	b.metadata.created = created
	return b
}

func (b *ChatResponseMetadataBuilder) WithKeyValue(key string, value any) *ChatResponseMetadataBuilder {
	b.metadata.metadata.Put(key, value)
	return b
}

func (b *ChatResponseMetadataBuilder) WithMetadata(metadata map[string]any) *ChatResponseMetadataBuilder {
	b.metadata.metadata.PutAll(kv.As(metadata))
	return b
}

func (b *ChatResponseMetadataBuilder) Build() *ChatResponseMetadata {
	return b.metadata
}
