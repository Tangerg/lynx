package model

import (
	"github.com/Tangerg/lynx/ai/core/embedding/request"
	"github.com/Tangerg/lynx/ai/core/embedding/response"
	"github.com/Tangerg/lynx/ai/core/model"
)

type EmbeddingModel[O request.EmbeddingRequestOptions] interface {
	model.Model[*request.EmbeddingRequest[O], *response.EmbeddingResponse]
}
