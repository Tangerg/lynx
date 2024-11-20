package request

import "github.com/Tangerg/lynx/ai/core/model"

type ImageRequestOptions interface {
	model.RequestOptions
	N() int
	Model() string
	Width() int
	Height() int
	ResponseFormat() string
	Style() string
}
