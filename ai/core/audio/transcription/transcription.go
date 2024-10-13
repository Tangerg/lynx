package transcription

import (
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Result[string, Metadata] = (*Transcription[Metadata])(nil)

type Transcription[M Metadata] struct {
	text     string
	metadata M
}

func (t *Transcription[M]) Output() string {
	return t.text
}

func (t *Transcription[M]) Metadata() M {
	return t.metadata
}
