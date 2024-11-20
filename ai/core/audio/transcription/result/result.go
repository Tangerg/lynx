package result

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Result[string, *TranscriptionResultMetadata] = (*TranscriptionResult[*TranscriptionResultMetadata])(nil)

type TranscriptionResult struct {
	metadata *TranscriptionResultMetadata
	text     string
}

func NewTranscriptionResult(text string, metadata *TranscriptionResultMetadata) *TranscriptionResult {
	return &TranscriptionResult{
		text:     text,
		metadata: metadata,
	}
}

func (t *TranscriptionResult) Output() string {
	return t.text
}

func (t *TranscriptionResult) Metadata() *TranscriptionResultMetadata {
	return t.metadata
}
