package embedding

import "github.com/Tangerg/lynx/ai/core/embedding/request"

var _ request.EmbeddingRequestOptions = (*OpenAIEmbeddingRequestOptions)(nil)

type OpenAIEmbeddingRequestOptions struct {
	model          string
	encodingFormat string
	dimensions     int
	user           string
}

func (o *OpenAIEmbeddingRequestOptions) Model() string {
	return o.model
}

func (o *OpenAIEmbeddingRequestOptions) Dimensions() int {
	return o.dimensions
}
func (o *OpenAIEmbeddingRequestOptions) EncodingFormat() string {
	return o.encodingFormat
}
func (o *OpenAIEmbeddingRequestOptions) User() string {
	return o.user
}
