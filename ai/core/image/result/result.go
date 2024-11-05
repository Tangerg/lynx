package result

import (
	"github.com/Tangerg/lynx/ai/core/image/image"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Result[*image.Image, ImageResultMetadata] = (*ImageResult[ImageResultMetadata])(nil)

type ImageResult[M ImageResultMetadata] struct {
	metadata M
	image    *image.Image
}

func (i *ImageResult[M]) Output() *image.Image {
	return i.image
}

func (i *ImageResult[M]) Metadata() M {
	return i.metadata
}
