package response

import (
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.ResponseMetadata = (*EmbeddingResponseMetadata)(nil)

type EmbeddingResponseMetadata struct {
	model.ResponseMetadata
}
