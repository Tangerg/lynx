package image

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Request[[]*Message, Options] = (*Prompt[Options])(nil)

type Prompt[O Options] struct {
	options  O
	messages []*Message
}

func (p *Prompt[O]) Instructions() []*Message {
	return p.messages
}

func (p *Prompt[O]) Options() O {
	return p.options
}
