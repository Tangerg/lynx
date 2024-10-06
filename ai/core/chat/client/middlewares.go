package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type Middlewares[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Middlewares() []middleware.Middleware[O, M]
	Param(key string) (any, bool)
	Params() map[string]any
	SetMiddlewares(middlewares ...middleware.Middleware[O, M]) Middlewares[O, M]
	SetParam(k string, v any) Middlewares[O, M]
	SetParams(m map[string]any) Middlewares[O, M]
}

func NewDefaultMiddlewares[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *DefaultMiddlewares[O, M] {
	return &DefaultMiddlewares[O, M]{
		params: make(map[string]any),
	}
}

var _ Middlewares[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultMiddlewares[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultMiddlewares[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	middlewares []middleware.Middleware[O, M]
	params      map[string]any
}

func (d *DefaultMiddlewares[O, M]) Middlewares() []middleware.Middleware[O, M] {
	return d.middlewares
}

func (d *DefaultMiddlewares[O, M]) SetMiddlewares(middlewares ...middleware.Middleware[O, M]) Middlewares[O, M] {
	d.middlewares = append(d.middlewares, middlewares...)
	return d
}

func (d *DefaultMiddlewares[O, M]) SetParam(k string, v any) Middlewares[O, M] {
	d.params[k] = v
	return d
}

func (d *DefaultMiddlewares[O, M]) SetParams(m map[string]any) Middlewares[O, M] {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}

func (d *DefaultMiddlewares[O, M]) Param(key string) (any, bool) {
	v, ok := d.params[key]
	return v, ok
}

func (d *DefaultMiddlewares[O, M]) Params() map[string]any {
	return d.params
}
