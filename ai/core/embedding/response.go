package embedding

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.Response[[]float64, *ResultMetadata] = (*Response)(nil)

type Response struct {
	embeddings []model.Result[[]float64, *ResultMetadata]
	metadata   *ResponseMetadata
}

func (r *Response) Result() model.Result[[]float64, *ResultMetadata] {
	if len(r.embeddings) == 0 {
		return nil
	}
	return r.embeddings[0]
}

func (r *Response) Results() []model.Result[[]float64, *ResultMetadata] {
	return r.embeddings
}

func (r *Response) Metadata() model.ResponseMetadata {
	return r.metadata
}

func NewResponse(embeddings []*Embedding, metadata *ResponseMetadata) *Response {
	rv := &Response{
		embeddings: make([]model.Result[[]float64, *ResultMetadata], 0, len(embeddings)),
		metadata:   metadata,
	}
	for _, embedding := range embeddings {
		rv.embeddings = append(rv.embeddings, embedding)
	}
	return rv
}
