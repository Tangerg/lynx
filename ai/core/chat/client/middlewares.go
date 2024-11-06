package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// Middlewares is a generic interface that defines the contract for managing middleware functions
// and their associated parameters in a chat application. It is parameterized by chat options (O)
// and chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Methods:
//
// Middlewares() []middleware.Middleware[O, M]
//   - Returns a slice of middleware functions currently set for the chat application.
//   - This method provides access to the list of middleware functions that will be executed during request processing.
//
// Param(key string) (any, bool)
//   - Retrieves a parameter value associated with the specified key.
//   - Returns the value and a boolean indicating whether the key was found in the parameters map.
//
// Params() map[string]any
//   - Returns a map of all parameters currently set for the middleware.
//   - This method provides access to all key-value pairs used to configure the middleware functions.
//
// SetMiddlewares(middlewares ...middleware.Middleware[O, M]) Middlewares[O, M]
//   - Sets the middleware functions for the chat application.
//   - Returns the Middlewares instance to allow method chaining.
//
// SetParam(k string, v any) Middlewares[O, M]
//   - Sets a single parameter key-value pair for the middleware configuration.
//   - Returns the Middlewares instance to allow method chaining.
//
// SetParams(m map[string]any) Middlewares[O, M]
//   - Sets multiple parameters using a map of key-value pairs for the middleware configuration.
//   - Returns the Middlewares instance to allow method chaining.
type Middlewares[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	Middlewares() []middleware.Middleware[O, M]
	Param(key string) (any, bool)
	Params() map[string]any
	SetMiddlewares(middlewares ...middleware.Middleware[O, M]) Middlewares[O, M]
	SetParam(k string, v any) Middlewares[O, M]
	SetParams(m map[string]any) Middlewares[O, M]
}

func NewDefaultMiddlewares[O request.ChatRequestOptions, M result.ChatResultMetadata]() *DefaultMiddlewares[O, M] {
	return &DefaultMiddlewares[O, M]{
		params: make(map[string]any),
	}
}

var _ Middlewares[request.ChatRequestOptions, result.ChatResultMetadata] = (*DefaultMiddlewares[request.ChatRequestOptions, result.ChatResultMetadata])(nil)

type DefaultMiddlewares[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
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
