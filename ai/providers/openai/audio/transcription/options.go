package transcription

import "github.com/Tangerg/lynx/ai/core/audio/transcription/request"

var _ request.TranscriptionRequestOptions = (*OpenAITranscodingRequestOptions)(nil)

type OpenAITranscodingRequestOptions struct {
	model                  string
	language               string
	prompt                 string
	responseFormat         string
	temperature            float64
	timestampGranularities []string
}

func (o *OpenAITranscodingRequestOptions) Model() string {
	return o.model
}

func (o *OpenAITranscodingRequestOptions) Language() string {
	return o.language
}

func (o *OpenAITranscodingRequestOptions) Prompt() string {
	return o.prompt
}

func (o *OpenAITranscodingRequestOptions) ResponseFormat() string {
	return o.responseFormat
}

func (o *OpenAITranscodingRequestOptions) Temperature() float64 {
	return o.temperature
}

func (o *OpenAITranscodingRequestOptions) TimestampGranularities() []string {
	return o.timestampGranularities
}
