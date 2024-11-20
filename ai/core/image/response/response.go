package response

import (
	"github.com/Tangerg/lynx/ai/core/image/image"
	"github.com/Tangerg/lynx/ai/core/image/result"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Response[*image.Image, result.ImageResultMetadata] = (*ImageResponse[result.ImageResultMetadata])(nil)

type ImageResponse[M result.ImageResultMetadata] struct {
	metadata *ImageResponseMetadata
	results  []model.Result[*image.Image, M]
}

func (r *ImageResponse[M]) Result() model.Result[*image.Image, M] {
	if len(r.results) == 0 {
		return nil
	}
	return r.results[0]
}

func (r *ImageResponse[M]) Results() []model.Result[*image.Image, M] {
	return r.results
}

func (r *ImageResponse[M]) Metadata() model.ResponseMetadata {
	return r.metadata
}

func NewImageResponse[M result.ImageResultMetadata](images []*result.ImageResult[M], metadata *ImageResponseMetadata) *ImageResponse[M] {
	rv := &ImageResponse[M]{
		metadata: metadata,
		results:  make([]model.Result[*image.Image, M], 0, len(images)),
	}
	for _, img := range images {
		rv.results = append(rv.results, img)
	}
	return rv
}
