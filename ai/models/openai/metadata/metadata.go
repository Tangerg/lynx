package metadata

import (
	"github.com/sashabaranov/go-openai"

	chatMetadata "github.com/Tangerg/lynx/ai/core/chat/metadata"
)

var _ chatMetadata.GenerationMetadata = (*OpenAIChatGenerationMetadata)(nil)

type OpenAIChatGenerationMetadata struct {
	finishReason string
	usage        *OpenAIUsage
}

func (o *OpenAIChatGenerationMetadata) FinishReason() string {
	return o.finishReason
}

func (o *OpenAIChatGenerationMetadata) Usage() chatMetadata.Usage {
	return o.usage
}

type OpenAIChatGenerationMetadataBuilder struct {
	metadata *OpenAIChatGenerationMetadata
}

func NewOpenAIChatGenerationMetadataBuilder() *OpenAIChatGenerationMetadataBuilder {
	return &OpenAIChatGenerationMetadataBuilder{
		metadata: &OpenAIChatGenerationMetadata{},
	}
}

func (b *OpenAIChatGenerationMetadataBuilder) FromCompletionResponse(resp *openai.CompletionResponse) *OpenAIChatGenerationMetadataBuilder {
	return b
}

func (b *OpenAIChatGenerationMetadataBuilder) WithFinishReason(reason string) *OpenAIChatGenerationMetadataBuilder {
	b.metadata.finishReason = reason
	return b
}
func (b *OpenAIChatGenerationMetadataBuilder) WithUsage(usage *OpenAIUsage) *OpenAIChatGenerationMetadataBuilder {
	b.metadata.usage = usage
	return b
}

func (b *OpenAIChatGenerationMetadataBuilder) Build() *OpenAIChatGenerationMetadata {
	return b.metadata
}
