package response

import (
	"github.com/Tangerg/lynx/ai/core/audio/transcription/result"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Response[string, result.TranscriptionResultMetadata] = (*TranscriptionResponse[result.TranscriptionResultMetadata])(nil)

type TranscriptionResponse struct {
	metadata   *TranscriptionResponseMetadata
	transcript model.Result[string, result.TranscriptionResultMetadata]
}

func (r *TranscriptionResponse) Result() model.Result[string, result.TranscriptionResultMetadata] {
	return r.transcript
}

func (r *TranscriptionResponse) Results() []model.Result[string, result.TranscriptionResultMetadata] {
	return []model.Result[string, result.TranscriptionResultMetadata]{r.transcript}
}

func (r *TranscriptionResponse) Metadata() model.ResponseMetadata {
	return r.metadata
}
