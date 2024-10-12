package image

import (
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Response[*Image, GenerationMetadata] = (*Response[GenerationMetadata])(nil)

type Response[GM GenerationMetadata] struct {
	metadata    *ResponseMetadata
	generations []model.Result[*Image, GM]
}

func (r *Response[GM]) Result() model.Result[*Image, GM] {
	if len(r.generations) == 0 {
		return nil
	}
	return r.generations[0]
}

func (r *Response[GM]) Results() []model.Result[*Image, GM] {
	return r.generations
}

func (r *Response[GM]) Metadata() model.ResponseMetadata {
	return r.metadata
}
