package chat

import (
	"context"
	"github.com/Tangerg/lynx/ai/model/chat/model"
	"maps"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat/request"
)

type Request struct {
	ctx         context.Context
	fields      map[string]any
	mu          sync.RWMutex
	chatModel   model.ChatModel
	chatRequest *request.ChatRequest
}

// NewRequest todo fix it to all fill
func NewRequest(ctx context.Context, options *Options) *Request {
	return &Request{}
}

func (c *Request) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *Request) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.fields[key] = value
}

func (c *Request) SetMap(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	maps.Copy(c.fields, m)
}

func (c *Request) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.fields[key]
	return val, ok
}

func (c *Request) ChatRequest() *request.ChatRequest {
	return c.chatRequest
}

func (c *Request) ChatModel() model.ChatModel {
	return c.chatModel
}
