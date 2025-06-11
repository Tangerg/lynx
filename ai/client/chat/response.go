package chat

import (
	"maps"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat/response"
)

type Response struct {
	mu           sync.RWMutex
	fields       map[string]any
	chatResponse *response.ChatResponse
}

func NewResponse(chatResponse *response.ChatResponse) *Response {
	return &Response{
		chatResponse: chatResponse,
		fields:       make(map[string]any),
	}
}

func (c *Response) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fields[key] = value
}

func (c *Response) SetMap(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.Copy(c.fields, m)
}

func (c *Response) Get(key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.fields[key]
	return val, ok
}

func (c *Response) ChatResponse() *response.ChatResponse {
	return c.chatResponse
}

func (c *Response) String() string {
	return ""
}

type StructuredResponse[T any] struct {
	data     T
	response *Response
}

func newStructuredResponse[T any](data T, response *Response) *StructuredResponse[T] {
	return &StructuredResponse[T]{
		data:     data,
		response: response,
	}
}

func (r *StructuredResponse[T]) Data() T {
	return r.data
}

func (r *StructuredResponse[T]) Response() *Response {
	return r.response
}
