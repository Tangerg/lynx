package embedding

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
)

type ResponseMetadata struct {
	Model     string
	Usage     *chat.Usage
	RateLimit *chat.RateLimit
	Created   int64
	Extra     map[string]any
}

func (r *ResponseMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

func (r *ResponseMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	v, ok := r.Extra[key]
	return v, ok
}

func (r *ResponseMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

var _ model.Response[*Result, *ResponseMetadata] = (*Response)(nil)

type Response struct {
	results  []*Result
	metadata *ResponseMetadata
}

func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("results at least one result required")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}
	return &Response{
		results:  results,
		metadata: metadata,
	}, nil
}

func (r *Response) Result() *Result {
	if len(r.results) > 0 {
		return r.results[0]
	}
	return nil
}

func (r *Response) Results() []*Result {
	return r.results
}

func (r *Response) Metadata() *ResponseMetadata {
	return r.metadata
}
