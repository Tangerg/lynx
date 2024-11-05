package request

import (
	"github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

var _ model.Request[*media.Media, TranscriptionRequestOptions] = (*TranscriptionRequest[TranscriptionRequestOptions])(nil)

type TranscriptionRequest[O TranscriptionRequestOptions] struct {
	options O
	audio   *media.Media
}

func (r *TranscriptionRequest[O]) Instructions() *media.Media {
	return r.audio
}

func (r *TranscriptionRequest[O]) Options() O {
	return r.options
}
