package transcription

import (
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Response[string, Metadata] = (*Response[Metadata])(nil)

type Response[RM Metadata] struct {
	metadata   ResponseMetadata
	transcript model.Result[string, RM]
}

func (r *Response[RM]) Result() model.Result[string, RM] {
	return r.transcript
}

func (r *Response[RM]) Results() []model.Result[string, RM] {
	return []model.Result[string, RM]{r.transcript}
}

func (r *Response[RM]) Metadata() model.ResponseMetadata {
	return r.metadata
}
