package request

import "github.com/Tangerg/lynx/ai/core/model"

type EmbeddingRequestOptions interface {
	model.RequestOptions
	Model() string
	Dimensions() int
}
