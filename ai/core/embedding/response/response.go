package response

import (
	"github.com/Tangerg/lynx/ai/core/embedding/result"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Response[[]float64, *result.EmbeddingResultMetadata] = (*EmbeddingResponse)(nil)

type EmbeddingResponse struct {
	metadata   *EmbeddingResponseMetadata
	embeddings []model.Result[[]float64, *result.EmbeddingResultMetadata]
}

func (r *EmbeddingResponse) Result() model.Result[[]float64, *result.EmbeddingResultMetadata] {
	if len(r.embeddings) == 0 {
		return nil
	}
	return r.embeddings[0]
}

func (r *EmbeddingResponse) Results() []model.Result[[]float64, *result.EmbeddingResultMetadata] {
	return r.embeddings
}

func (r *EmbeddingResponse) Metadata() model.ResponseMetadata {
	return r.metadata
}

func NewEmbeddingResponse(embeddings []*result.EmbeddingResult, metadata *EmbeddingResponseMetadata) *EmbeddingResponse {
	rv := &EmbeddingResponse{
		embeddings: make([]model.Result[[]float64, *result.EmbeddingResultMetadata], 0, len(embeddings)),
		metadata:   metadata,
	}
	for _, embedding := range embeddings {
		rv.embeddings = append(rv.embeddings, embedding)
	}
	return rv
}
