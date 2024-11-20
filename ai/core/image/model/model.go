package model

import (
	"github.com/Tangerg/lynx/ai/core/image/request"
	"github.com/Tangerg/lynx/ai/core/image/response"
	"github.com/Tangerg/lynx/ai/core/image/result"
	"github.com/Tangerg/lynx/ai/core/model"
)

type ImageModel[O request.ImageRequestOptions, M result.ImageResultMetadata] interface {
	model.Model[*request.ImageRequest[O], *response.ImageResponse[M]]
}
