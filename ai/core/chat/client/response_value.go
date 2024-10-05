package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
)

type ResponseValue[T any, M metadata.ChatGenerationMetadata] interface {
	Value() T
	Content() string
	Response() *completion.ChatCompletion[M]
}

func NewDefaultResponseValue[T any, M metadata.ChatGenerationMetadata](value T) *DefaultResponseValue[T, M] {
	r := &DefaultResponseValue[T, M]{
		value: value,
	}
	return r
}

var _ ResponseValue[any, metadata.ChatGenerationMetadata] = (*DefaultResponseValue[any, metadata.ChatGenerationMetadata])(nil)

type DefaultResponseValue[T any, M metadata.ChatGenerationMetadata] struct {
	value    T
	response *completion.ChatCompletion[M]
}

func (e *DefaultResponseValue[T, M]) Value() T {
	return e.value
}

func (e *DefaultResponseValue[T, M]) Content() string {
	return e.response.Result().Output().Content()
}

func (e *DefaultResponseValue[T, M]) Response() *completion.ChatCompletion[M] {
	return e.response
}
