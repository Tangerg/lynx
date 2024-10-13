package transcription

import (
	"github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

var _ model.Request[*media.Media, Options] = (*Prompt[Options])(nil)

type Prompt[O Options] struct {
	audio   *media.Media
	options O
}

func (p *Prompt[O]) Instructions() *media.Media {
	return p.audio
}

func (p *Prompt[O]) Options() O {
	return p.options
}
