package metadata

//
//import (
//	"github.com/Tangerg/lynx/ai/core/chat/result"
//)
//
//var _ result.ChatResultMetadata = (*OpenAIChatGenerationMetadata)(nil)
//
//type OpenAIChatGenerationMetadata struct {
//	finishReason result.FinishReason
//}
//
//func (o *OpenAIChatGenerationMetadata) FinishReason() result.FinishReason {
//	return o.finishReason
//}
//
//type OpenAIChatGenerationMetadataBuilder struct {
//	metadata *OpenAIChatGenerationMetadata
//}
//
//func NewOpenAIChatGenerationMetadataBuilder() *OpenAIChatGenerationMetadataBuilder {
//	return &OpenAIChatGenerationMetadataBuilder{
//		metadata: &OpenAIChatGenerationMetadata{},
//	}
//}
//
//func (b *OpenAIChatGenerationMetadataBuilder) WithFinishReason(reason result.FinishReason) *OpenAIChatGenerationMetadataBuilder {
//	b.metadata.finishReason = reason
//	return b
//}
//
//func (b *OpenAIChatGenerationMetadataBuilder) Build() *OpenAIChatGenerationMetadata {
//	return b.metadata
//}
