package result

import (
	"errors"
	"github.com/Tangerg/lynx/ai/core/chat/message"
)

type ChatResult[M ChatResultMetadata] struct {
	message  *message.AssistantMessage
	metadata M
}

func (g *ChatResult[M]) Output() *message.AssistantMessage {
	return g.message
}

func (g *ChatResult[M]) Metadata() M {
	return g.metadata
}

type ChatResultBuilder[M ChatResultMetadata] struct {
	result *ChatResult[M]
}

func NewChatResultBuilder[M ChatResultMetadata]() *ChatResultBuilder[M] {
	return &ChatResultBuilder[M]{}
}

func (b *ChatResultBuilder[M]) WithContent(content string) *ChatResultBuilder[M] {
	return b.WithMessage(message.NewAssistantMessage(content, nil, nil))
}

func (b *ChatResultBuilder[M]) WithMessage(msg *message.AssistantMessage) *ChatResultBuilder[M] {
	b.result.message = msg
	return b
}

func (b *ChatResultBuilder[M]) WithMetadata(meta M) *ChatResultBuilder[M] {
	b.result.metadata = meta
	return b
}

func (b *ChatResultBuilder[M]) Build() (*ChatResult[M], error) {
	if b.result.message == nil {
		return nil, errors.New("message is nil")
	}
	return b.result, nil
}
