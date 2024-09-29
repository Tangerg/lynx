package prompt

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
)

type Prompt[O Options] struct {
	messages []message.Message
	options  O
}

func (p *Prompt[O]) Instructions() []message.Message {
	return p.messages
}

func (p *Prompt[O]) Options() O {
	return p.options
}

type Builder[O Options] struct {
	prompt *Prompt[O]
}

func NewBuilder[O Options]() *Builder[O] {
	return &Builder[O]{
		prompt: &Prompt[O]{
			messages: make([]message.Message, 0),
		},
	}
}

func (b *Builder[O]) WithContent(content string) *Builder[O] {
	b.WithMessages(message.NewUserMessage(content))
	return b
}

func (b *Builder[O]) WithMessages(msg ...message.Message) *Builder[O] {
	b.prompt.messages = append(b.prompt.messages, msg...)
	return b
}

func (b *Builder[O]) WithOptions(opts O) *Builder[O] {
	b.prompt.options = opts
	return b
}

func (b *Builder[O]) Build() *Prompt[O] {
	return b.prompt
}
