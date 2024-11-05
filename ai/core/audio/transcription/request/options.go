package request

import "github.com/Tangerg/lynx/ai/core/model"

type TranscriptionRequestOptions interface {
	model.RequestOptions
	Model() string
}
