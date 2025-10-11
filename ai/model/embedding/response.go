package embedding

import (
	"errors"

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

type Response struct {
	Results  []*Result         `json:"results"`
	Metadata *ResponseMetadata `json:"metadata"`
}

func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("embedding response requires at least one result")
	}
	if metadata == nil {
		return nil, errors.New("embedding response metadata is required")
	}
	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

func (c *Response) Result() *Result {
	if len(c.Results) > 0 {
		return c.Results[0]
	}
	return nil
}
