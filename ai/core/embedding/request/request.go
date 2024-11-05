package request

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Request[[]string, EmbeddingRequestOptions] = (*EmbeddingRequest[EmbeddingRequestOptions])(nil)

type EmbeddingRequest[O EmbeddingRequestOptions] struct {
	inputs  []string
	options O
}

func (r *EmbeddingRequest[O]) Instructions() []string {
	return r.inputs
}

func (r *EmbeddingRequest[O]) Options() O {
	return r.options
}

func NewEmbeddingRequest[O EmbeddingRequestOptions](inputs []string, o O) *EmbeddingRequest[O] {
	return &EmbeddingRequest[O]{
		inputs:  inputs,
		options: o,
	}
}
