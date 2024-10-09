package embedding

import "github.com/Tangerg/lynx/ai/core/embedding"

var _ embedding.Options = (*OpenAIEmbeddingOptions)(nil)

type OpenAIEmbeddingOptions struct {
	model          string
	encodingFormat string
	dimensions     int
	user           string
}

func (options *OpenAIEmbeddingOptions) Model() string {
	return options.model
}
func (options *OpenAIEmbeddingOptions) EncodingFormat() string {
	return options.encodingFormat
}
func (options *OpenAIEmbeddingOptions) Dimensions() int {
	return options.dimensions
}
func (options *OpenAIEmbeddingOptions) User() string {
	return options.user
}
