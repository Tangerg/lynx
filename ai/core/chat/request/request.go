package request

import (
	"errors"

	"github.com/Tangerg/lynx/ai/core/chat/message"
)

type ChatRequest[O ChatRequestOptions] struct {
	messages []message.ChatMessage
	options  O
}

func (p *ChatRequest[O]) Instructions() []message.ChatMessage {
	return p.messages
}

func (p *ChatRequest[O]) Options() O {
	return p.options
}

type ChatRequestBuilder[O ChatRequestOptions] struct {
	prompt *ChatRequest[O]
}

func NewChatRequestBuilder[O ChatRequestOptions]() *ChatRequestBuilder[O] {
	return &ChatRequestBuilder[O]{
		prompt: &ChatRequest[O]{
			messages: make([]message.ChatMessage, 0),
		},
	}
}

func (b *ChatRequestBuilder[O]) WithContent(content string) *ChatRequestBuilder[O] {
	b.WithMessages(message.NewUserMessage(content, nil))
	return b
}

func (b *ChatRequestBuilder[O]) WithMessages(msg ...message.ChatMessage) *ChatRequestBuilder[O] {
	b.prompt.messages = msg
	return b
}

func (b *ChatRequestBuilder[O]) WithOptions(opts O) *ChatRequestBuilder[O] {
	b.prompt.options = opts
	return b
}

func (b *ChatRequestBuilder[O]) Build() (*ChatRequest[O], error) {
	if b.prompt.options.Model() == nil {
		return nil, errors.New("no options model")
	}
	if b.prompt.messages == nil || len(b.prompt.messages) == 0 {
		return nil, errors.New("no messages")
	}
	return b.prompt, nil
}
