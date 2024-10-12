package image

import "github.com/Tangerg/lynx/ai/core/model"

var _ model.ResponseMetadata = (*ResponseMetadata)(nil)

type ResponseMetadata struct {
	metadata map[string]any
}

func (r *ResponseMetadata) Get(key string) (any, bool) {
	val, ok := r.metadata[key]
	return val, ok
}

func (r *ResponseMetadata) GetOrDefault(key string, def any) any {
	val, ok := r.metadata[key]
	if !ok {
		val = def
	}
	return val
}
