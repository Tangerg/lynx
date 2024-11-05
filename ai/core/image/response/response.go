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
