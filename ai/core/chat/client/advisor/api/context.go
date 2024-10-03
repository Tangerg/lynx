package api

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
)

type Context struct {
	ctx      context.Context
	params   map[string]any
	Request  *AdvisedRequest
	Response *completion.ChatCompletion[metadata.ChatGenerationMetadata]
}

func NewContext(ctx context.Context) *Context {
	return &Context{
		ctx:    ctx,
		params: make(map[string]any),
	}
}

func (c *Context) Context() context.Context {
	return c.ctx
}

func (c *Context) Param(key string) (any, bool) {
	v, ok := c.params[key]
	return v, ok
}

func (c *Context) Params() map[string]any {
	return c.params
}

func (c *Context) SetParam(key string, value any) {
	c.params[key] = value
}

func (c *Context) SetParams(m map[string]any) {
	for k, v := range m {
		c.params[k] = v
	}
}
