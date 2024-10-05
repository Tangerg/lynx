package api

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

func NewContext[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](ctx context.Context) *Context[O, M] {
	return &Context[O, M]{
		ctx:    ctx,
		params: make(map[string]any),
	}
}

type Context[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	ctx      context.Context
	params   map[string]any
	Request  *AdvisedRequest[O, M]
	Response *completion.ChatCompletion[M]
}

func (c *Context[O, M]) Context() context.Context {
	return c.ctx
}

func (c *Context[O, M]) Param(key string) (any, bool) {
	v, ok := c.params[key]
	return v, ok
}

func (c *Context[O, M]) Params() map[string]any {
	return c.params
}

func (c *Context[O, M]) SetParam(key string, value any) {
	c.params[key] = value
}

func (c *Context[O, M]) SetParams(m map[string]any) {
	for k, v := range m {
		c.params[k] = v
	}
}
