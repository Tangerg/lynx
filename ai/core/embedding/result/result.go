package result

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Result[[]float64, *EmbeddingResultMetadata] = (*EmbeddingResult)(nil)

type EmbeddingResult struct {
	embedding []float64
	index     int
	metadata  *EmbeddingResultMetadata
}

func (e *EmbeddingResult) Output() []float64 {
	return e.embedding
}

func (e *EmbeddingResult) Metadata() *EmbeddingResultMetadata {
	return e.metadata
}

func (e *EmbeddingResult) Index() int {
	return e.index
}

func NewEmbedding(embedding []float64, index int, metadata *EmbeddingResultMetadata) *EmbeddingResult {
	return &EmbeddingResult{
		embedding: embedding,
		index:     index,
		metadata:  metadata,
	}
}
