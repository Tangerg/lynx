package chat

import (
	"maps"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat/response"
)

type Response struct {
	mu           sync.RWMutex
	metadata     map[string]any
	chatResponse *response.ChatResponse
}

func NewResponse(chatResponse *response.ChatResponse) *Response {
	return &Response{
		chatResponse: chatResponse,
		metadata:     make(map[string]any),
	}
}

func (c *Response) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metadata[key] = value
}

func (c *Response) SetMap(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.Copy(c.metadata, m)
}

func (c *Response) Get(key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.metadata[key]
	return val, ok
}

func (c *Response) ChatResponse() *response.ChatResponse {
	return c.chatResponse
}
