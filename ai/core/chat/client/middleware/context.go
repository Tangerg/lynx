package middleware

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// Context is a generic struct that manages the state and flow of chat processing
// through a sequence of middleware functions. It is parameterized by chat options (O)
// and chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, typically defined by request.ChatRequestOptions.
//   - M: Represents the metadata associated with chat generation, typically defined
//     by result.ChatResultMetadata.
//
// Fields:
//   - ctx: The base context.Context instance, used for managing request-scoped values,
//     deadlines, and cancellation signals.
//   - mu: A sync.RWMutex to ensure thread-safe access to the params map.
//   - params: A map for storing arbitrary key-value pairs, enabling middleware to share data.
//   - middlewareIndex: An integer tracking the current position in the middleware execution chain.
//   - middlewares: A slice of Middleware[O, M] functions, executed in sequence.
//   - Request: A pointer to a Request[O, M] struct, representing the incoming chat request.
//   - Response: A pointer to a response.ChatResponse[M] struct, representing the generated response.
type Context[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	ctx             context.Context
	mu              sync.RWMutex
	params          map[string]any
	middlewareIndex int
	middlewares     []Middleware[O, M]
	Request         *Request[O, M]
	Response        *response.ChatResponse[M]
}

func (c *Context[O, M]) Context() context.Context {
	return c.ctx
}

// Next advances to the next middleware in the execution chain.
// If no more middleware functions are available, it simply returns nil.
//
// Returns:
//   - error: If the current middleware encounters an error, it is returned.
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

// NewContext creates a new Context instance with an initialized middleware chain
// and an empty params map.
//
// Parameters:
//   - ctx: The base context.Context instance for managing request-scoped values.
//
// Returns:
//   - *Context[O, M]: A new Context instance ready for middleware processing.
func NewContext[O request.ChatRequestOptions, M result.ChatResultMetadata](ctx context.Context) *Context[O, M] {
	return &Context[O, M]{
		ctx:             ctx,
		params:          make(map[string]any),
		middlewareIndex: -1,
		middlewares:     make([]Middleware[O, M], 0),
	}
}
