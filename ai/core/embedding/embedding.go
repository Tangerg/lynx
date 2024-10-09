package embedding

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Result[[]float64, *ResultMetadata] = (*Embedding)(nil)

type Embedding struct {
	embedding []float64
	index     int
	metadata  *ResultMetadata
}

func (e *Embedding) Output() []float64 {
	return e.embedding
}

func (e *Embedding) Metadata() *ResultMetadata {
	return e.metadata
}

func (e *Embedding) Index() int {
	return e.index
}

func NewEmbedding(embedding []float64, index int, metadata *ResultMetadata) *Embedding {
	return &Embedding{
		embedding: embedding,
		index:     index,
		metadata:  metadata,
	}
}
