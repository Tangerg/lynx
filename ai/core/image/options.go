package image

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Options = (*Options)(nil)

type Options interface {
	N() int64
	Model() string
	Width() int
	Height() int
	ResponseFormat() string
	Style() string
}
