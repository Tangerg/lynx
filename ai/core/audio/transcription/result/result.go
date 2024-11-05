package result

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Result[string, TranscriptionResultMetadata] = (*TranscriptionResult[TranscriptionResultMetadata])(nil)

type TranscriptionResult[M TranscriptionResultMetadata] struct {
	metadata M
	text     string
}

func (t *TranscriptionResult[M]) Output() string {
	return t.text
}

func (t *TranscriptionResult[M]) Metadata() M {
	return t.metadata
}
