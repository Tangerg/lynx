package metadata

import (
	chatMetadata "github.com/Tangerg/lynx/ai/core/chat/metadata"
)

var _ chatMetadata.ChatGenerationMetadata = (*OpenAIChatGenerationMetadata)(nil)

type OpenAIChatGenerationMetadata struct {
	finishReason chatMetadata.FinishReason
}

func (o *OpenAIChatGenerationMetadata) FinishReason() chatMetadata.FinishReason {
	return o.finishReason
}

type OpenAIChatGenerationMetadataBuilder struct {
	metadata *OpenAIChatGenerationMetadata
}

func NewOpenAIChatGenerationMetadataBuilder() *OpenAIChatGenerationMetadataBuilder {
	return &OpenAIChatGenerationMetadataBuilder{
		metadata: &OpenAIChatGenerationMetadata{},
	}
}

func (b *OpenAIChatGenerationMetadataBuilder) WithFinishReason(reason chatMetadata.FinishReason) *OpenAIChatGenerationMetadataBuilder {
	b.metadata.finishReason = reason
	return b
}

func (b *OpenAIChatGenerationMetadataBuilder) Build() *OpenAIChatGenerationMetadata {
	return b.metadata
}
