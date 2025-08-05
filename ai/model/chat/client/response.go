package client

import (
	"github.com/Tangerg/lynx/ai/model/chat"
)

type StructuredResponse[T any] struct {
	value        T
	chatResponse *chat.Response
}

func newStructuredResponse[T any](value T, response *chat.Response) *StructuredResponse[T] {
	return &StructuredResponse[T]{
		value:        value,
		chatResponse: response,
	}
}

func (s *StructuredResponse[T]) Value() T {
	return s.value
}

func (s *StructuredResponse[T]) ChatResponse() *chat.Response {
	return s.chatResponse
}
