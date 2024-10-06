package middleware

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

// Context is a generic struct that holds the state and controls the flow of chat processing
// through a series of middleware functions. It is parameterized by chat options (O) and
// chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Fields:
//   - ctx: An instance of context.Context, used for managing request-scoped values, cancellation signals, and deadlines.
//   - mu: A read-write mutex (sync.RWMutex) to ensure thread-safe access to the params map.
//   - params: A map for storing arbitrary key-value pairs, allowing middleware to share data.
//   - middlewareIndex: An integer tracking the current position in the middleware chain.
//   - middlewares: A slice of Middleware[O, M] functions that are executed in sequence.
//   - Request: A pointer to a Request[O, M] struct, representing the incoming chat request.
//   - Response: A pointer to a completion.ChatCompletion[M] struct, representing the chat response.
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

// Next
//   - Advances to the next middleware in the chain. If there are no more middlewares, it returns nil.
//   - Returns an error if the current middleware encounters an issue.
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

// SetMiddlewares
//   - Appends the provided middleware functions to the middlewares slice, allowing them to be executed in sequence.
func (c *Context[O, M]) SetMiddlewares(middlewares ...Middleware[O, M]) {
	c.middlewares = append(c.middlewares, middlewares...)
}

func NewContext[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](ctx context.Context) *Context[O, M] {
	return &Context[O, M]{
		ctx:             ctx,
		params:          make(map[string]any),
		middlewareIndex: -1,
		middlewares:     make([]Middleware[O, M], 0),
	}
}
