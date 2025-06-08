package chat

import (
	"context"
	"maps"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat/request"
)

type Context struct {
	ctx     context.Context
	fields  map[string]any
	mu      sync.RWMutex
	request *request.ChatRequest
}

func (c *Context) Context() context.Context {
	return c.ctx
}

func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.fields[key] = value
}

func (c *Context) SetMap(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	maps.Copy(c.fields, m)
}

func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.fields[key]
	return val, ok
}

func (c *Context) ChatRequest() *request.ChatRequest {
	return c.request
}

// todo
func newContextFromRequest(ctx context.Context, req *Request) *Context {
	return &Context{
		ctx: ctx,
	}
}
