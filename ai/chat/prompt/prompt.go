package prompt

import (
	"github.com/Tangerg/lynx/ai/chat/message"
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
