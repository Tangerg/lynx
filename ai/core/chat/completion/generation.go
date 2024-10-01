package completion

import (
	"errors"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Result[*message.AssistantMessage, metadata.GenerationMetadata] = (*Generation[metadata.GenerationMetadata])(nil)

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

func (b *GenerationBuilder[RM]) Build() (*Generation[RM], error) {
	if b.result.metadata == nil {
		return nil, errors.New("metadata is nil")
	}
	if b.result.message == nil {
		return nil, errors.New("message is nil")
	}
	return b.result, nil
}
