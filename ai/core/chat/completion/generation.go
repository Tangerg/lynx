package completion

import (
	"errors"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Result[*message.AssistantMessage, metadata.ChatGenerationMetadata] = (*ChatGeneration[metadata.ChatGenerationMetadata])(nil)

type ChatGeneration[M metadata.ChatGenerationMetadata] struct {
	message  *message.AssistantMessage
	metadata M
}

func (g *ChatGeneration[M]) Output() *message.AssistantMessage {
	return g.message
}

func (g *ChatGeneration[M]) Metadata() M {
	return g.metadata
}

type ChatGenerationBuilder[RM metadata.ChatGenerationMetadata] struct {
	result *ChatGeneration[RM]
}

func (b *ChatGenerationBuilder[RM]) WithContent(content string) *ChatGenerationBuilder[RM] {
	return b.WithMessage(message.NewAssistantMessage(content, nil, nil))
}

func (b *ChatGenerationBuilder[RM]) WithMessage(msg *message.AssistantMessage) *ChatGenerationBuilder[RM] {
	b.result.message = msg
	return b
}

func (b *ChatGenerationBuilder[RM]) WithMetadata(meta RM) *ChatGenerationBuilder[RM] {
	b.result.metadata = meta
	return b
}

func (b *ChatGenerationBuilder[RM]) Build() (*ChatGeneration[RM], error) {
	if b.result.message == nil {
		return nil, errors.New("message is nil")
	}
	return b.result, nil
}
