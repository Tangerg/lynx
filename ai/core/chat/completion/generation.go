package completion

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
)

type Generation[M metadata.GenerationMetadata] struct {
	message  *message.AssistantMessage
	metadata M
}

func (g *Generation[M]) Output() *message.AssistantMessage {
	return g.message
}

func (g *Generation[M]) Metadata() M {
	return g.metadata
}

type GenerationBuilder[RM metadata.GenerationMetadata] struct {
	result *Generation[RM]
}

func (b *GenerationBuilder[RM]) WithContent(content string) *GenerationBuilder[RM] {
	return b.WithMessage(message.NewAssistantMessage(content))
}

func (b *GenerationBuilder[RM]) WithMessage(msg *message.AssistantMessage) *GenerationBuilder[RM] {
	b.result.message = msg
	return b
}

func (b *GenerationBuilder[RM]) WithMetadata(meta RM) *GenerationBuilder[RM] {
	b.result.metadata = meta
	return b
}

func (b *GenerationBuilder[RM]) Build() *Generation[RM] {
	return b.result
}
