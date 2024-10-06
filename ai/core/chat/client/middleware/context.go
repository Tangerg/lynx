package middleware

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

func NewContext[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](ctx context.Context) *Context[O, M] {
	return &Context[O, M]{
		ctx:             ctx,
		params:          make(map[string]any),
		middlewareIndex: -1,
		middlewares:     make([]Middleware[O, M], 0),
	}
}

type Context[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	ctx             context.Context
	mu              sync.RWMutex
	params          map[string]any
	middlewareIndex int
	middlewares     []Middleware[O, M]
	Request         *Request[O, M]
	Response        *completion.ChatCompletion[M]
}

func (c *Context[O, M]) Context() context.Context {
	return c.ctx
}

func (c *Context[O, M]) Next() error {
	c.middlewareIndex++
	if c.middlewareIndex < len(c.middlewares) {
		return c.middlewares[c.middlewareIndex](c)
	}
	return nil
}

func (c *Context[O, M]) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.params[key]
	return val, ok
}

func (c *Context[O, M]) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.params[key] = value
}

func (c *Context[O, M]) SetMap(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range m {
		c.params[k] = v
	}
}

func (c *Context[O, M]) SetMiddlewares(middlewares ...Middleware[O, M]) {
	c.middlewares = append(c.middlewares, middlewares...)
}
