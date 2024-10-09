package embedding

import (
	chatMetadata "github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.ResponseMetadata = (*ResponseMetadata)(nil)

type ResponseMetadata struct {
	model    string
	usage    chatMetadata.Usage
	metadata map[string]any
}

func (r *ResponseMetadata) Get(key string) (any, bool) {
	v, ok := r.metadata[key]
	return v, ok
}

func (r *ResponseMetadata) GetOrDefault(key string, def any) any {
	v, ok := r.metadata[key]
	if !ok {
		v = def
	}
	return v
}

func (r *ResponseMetadata) Model() string {
	return r.model
}

func (r *ResponseMetadata) SetModel(model string) *ResponseMetadata {
	r.model = model
	return r
}

func (r *ResponseMetadata) Usage() chatMetadata.Usage {
	return r.usage
}

func (r *ResponseMetadata) SetUsage(usage chatMetadata.Usage) *ResponseMetadata {
	r.usage = usage
	return r
}

func (r *ResponseMetadata) SetParam(key string, value any) *ResponseMetadata {
	r.metadata[key] = value
	return r
}

func (r *ResponseMetadata) SetParams(m map[string]any) *ResponseMetadata {
	for k, v := range m {
		r.metadata[k] = v
	}
	return r
}

func NewResponseMetadata() *ResponseMetadata {
	return &ResponseMetadata{
		metadata: make(map[string]any),
		usage:    &chatMetadata.EmptyUsage{},
	}
}
