package response

type ChatResponseMetadata struct {
	id       string
	model    string
	metadata map[string]any
	usage    Usage
	created  int64
}

func (c *ChatResponseMetadata) Get(key string) (any, bool) {
	v, ok := c.metadata[key]
	return v, ok
}

func (c *ChatResponseMetadata) GetOrDefault(key string, def any) any {
	v, ok := c.metadata[key]
	if !ok {
		v = def
	}
	return v
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

func (c *ChatResponseMetadata) Created() int64 {
	return c.created
}

func newChatResponseMetadata() *ChatResponseMetadata {
	return &ChatResponseMetadata{
		metadata: make(map[string]any),
		usage:    &EmptyUsage{},
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

func (b *ChatResponseMetadataBuilder) WithCreated(created int64) *ChatResponseMetadataBuilder {
	b.metadata.created = created
	return b
}

func (b *ChatResponseMetadataBuilder) WithKeyValue(key string, value any) *ChatResponseMetadataBuilder {
	b.metadata.metadata[key] = value
	return b
}

func (b *ChatResponseMetadataBuilder) WithMetadata(metadata map[string]any) *ChatResponseMetadataBuilder {
	for k, v := range metadata {
		b.metadata.metadata[k] = v
	}
	return b
}

func (b *ChatResponseMetadataBuilder) Build() *ChatResponseMetadata {
	return b.metadata
}
