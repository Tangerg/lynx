package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// ResponseValue is a generic interface that defines the contract for accessing
// the value and related information of a chat response. It is parameterized by
// the type of the value (T) and chat generation metadata (M).
//
// Type Parameters:
//   - T: Represents the type of the value contained in the response.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Methods:
//
// Value() T
//   - Returns the value of the response, which is of type T.
//   - This method provides direct access to the underlying data contained in the response.
//
// Content() string
//   - Returns the content of the response as a string.
//   - This method is useful for obtaining a textual representation of the response, regardless of the underlying type.
//
// Response() *completion.ChatCompletion[M]
//   - Returns a pointer to the ChatCompletion, which contains the full details of the chat response.
//   - This method provides access to the complete response object, including metadata and other relevant information.
type ResponseValue[T any, M result.ChatResultMetadata] interface {
	Value() T
	Content() string
	Response() *response.ChatResponse[M]
}

func NewDefaultResponseValue[T any, M result.ChatResultMetadata](value T) *DefaultResponseValue[T, M] {
	r := &DefaultResponseValue[T, M]{
		value: value,
	}
	return r
}

var _ ResponseValue[any, result.ChatResultMetadata] = (*DefaultResponseValue[any, result.ChatResultMetadata])(nil)

type DefaultResponseValue[T any, M result.ChatResultMetadata] struct {
	value    T
	response *response.ChatResponse[M]
}

func (e *DefaultResponseValue[T, M]) Value() T {
	return e.value
}

func (e *DefaultResponseValue[T, M]) Content() string {
	return e.response.Result().Output().Content()
}

func (e *DefaultResponseValue[T, M]) Response() *response.ChatResponse[M] {
	return e.response
}
