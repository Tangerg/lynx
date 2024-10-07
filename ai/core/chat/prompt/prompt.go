package prompt

import (
	"errors"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Request[[]message.ChatMessage, ChatOptions] = (*ChatPrompt[ChatOptions])(nil)

type ChatPrompt[O ChatOptions] struct {
	messages []message.ChatMessage
	options  O
}

func (p *ChatPrompt[O]) Instructions() []message.ChatMessage {
	return p.messages
}

func (p *ChatPrompt[O]) Options() O {
	return p.options
}

type ChatPromptBuilder[O ChatOptions] struct {
	prompt *ChatPrompt[O]
}

func NewChatPromptBuilder[O ChatOptions]() *ChatPromptBuilder[O] {
	return &ChatPromptBuilder[O]{
		prompt: &ChatPrompt[O]{
			messages: make([]message.ChatMessage, 0),
		},
	}
}

func (b *ChatPromptBuilder[O]) WithContent(content string) *ChatPromptBuilder[O] {
	b.WithMessages(message.NewUserMessage(content, nil))
	return b
}

func (b *ChatPromptBuilder[O]) WithMessages(msg ...message.ChatMessage) *ChatPromptBuilder[O] {
	b.prompt.messages = msg
	return b
}

func (b *ChatPromptBuilder[O]) WithOptions(opts O) *ChatPromptBuilder[O] {
	b.prompt.options = opts
	return b
}

func (b *ChatPromptBuilder[O]) Build() (*ChatPrompt[O], error) {
	if b.prompt.options.Model() == nil {
		return nil, errors.New("no options model")
	}
	if b.prompt.messages == nil || len(b.prompt.messages) == 0 {
		return nil, errors.New("no messages")
	}
	return b.prompt, nil
}
