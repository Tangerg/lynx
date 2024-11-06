package chat

import "github.com/Tangerg/lynx/ai/core/chat/result"

var _ result.ChatResultMetadata = (*OpenAIChatResultMetadata)(nil)

type OpenAIChatResultMetadata struct {
	finishReason result.FinishReason
}

func (o *OpenAIChatResultMetadata) FinishReason() result.FinishReason {
	return o.finishReason
}

type OpenAIChatResultMetadataBuilder struct {
	metadata *OpenAIChatResultMetadata
}

func NewOpenAIChatResultMetadataBuilder() *OpenAIChatResultMetadataBuilder {
	return &OpenAIChatResultMetadataBuilder{
		metadata: &OpenAIChatResultMetadata{},
	}
}

func (b *OpenAIChatResultMetadataBuilder) WithFinishReason(reason result.FinishReason) *OpenAIChatResultMetadataBuilder {
	b.metadata.finishReason = reason
	return b
}

func (b *OpenAIChatResultMetadataBuilder) Build() *OpenAIChatResultMetadata {
	return b.metadata
}
