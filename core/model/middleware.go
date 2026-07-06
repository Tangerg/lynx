package model

import "slices"

// CallMiddleware wraps a [CallHandler] with cross-cutting behavior
// (logging, retry, caching, auth). Standard HTTP-middleware shape:
// take next, return a wrapping handler that delegates.
type CallMiddleware[Request any, Response any] func(next CallHandler[Request, Response]) CallHandler[Request, Response]

// StreamMiddleware wraps a [StreamHandler] with cross-cutting behavior.
// Behaves like [CallMiddleware] but operates over the streaming
// [iter.Seq2] result so it can intercept individual chunks.
type StreamMiddleware[Request any, Response any] func(next StreamHandler[Request, Response]) StreamHandler[Request, Response]

// MiddlewareChain keeps independent ordered call and stream middleware
// chains for a single [Request] / [Response] pair. Modalities that don't
// stream simply leave the stream side empty.
//
// Example:
//
//	chain := NewMiddlewareChain[*Req, *Resp]().
//	    WithCall(loggingMW, retryMW)
//	wrapped := chain.BuildCallHandler(rawHandler)
type MiddlewareChain[Request any, Response any] struct {
	call   []CallMiddleware[Request, Response]
	stream []StreamMiddleware[Request, Response]
}

// NewMiddlewareChain returns an empty chain. Type inference rarely works
// here, so call sites typically spell the two type parameters out.
func NewMiddlewareChain[Request any, Response any]() MiddlewareChain[Request, Response] {
	return MiddlewareChain[Request, Response]{}
}

// BuildCallHandler wraps endpoint with the registered call middleware
// chain and returns the composed handler. Middlewares run outer-first (the
// first registered is the outermost layer); on a cold cache call sequence
// for [mw1, mw2, mw3] the runtime path is mw1 → mw2 → mw3 → endpoint.
func (c MiddlewareChain[Request, Response]) BuildCallHandler(endpoint CallHandler[Request, Response]) CallHandler[Request, Response] {
	wrapped := endpoint
	for i := len(c.call) - 1; i >= 0; i-- {
		wrapped = c.call[i](wrapped)
	}
	return wrapped
}

// BuildStreamHandler wraps endpoint with the registered stream middleware
// chain. Composition order matches [MiddlewareChain.BuildCallHandler].
func (c MiddlewareChain[Request, Response]) BuildStreamHandler(endpoint StreamHandler[Request, Response]) StreamHandler[Request, Response] {
	wrapped := endpoint
	for i := len(c.stream) - 1; i >= 0; i-- {
		wrapped = c.stream[i](wrapped)
	}
	return wrapped
}

// WithCall replaces the call-side chain. nil entries are dropped so callers
// can pass optional middlewares without branching.
func (c MiddlewareChain[Request, Response]) WithCall(middlewares ...CallMiddleware[Request, Response]) MiddlewareChain[Request, Response] {
	c = c.Clone()
	c.call = compactCallMiddlewares(middlewares)
	return c
}

// WithStream replaces the stream-side chain. nil entries are dropped so
// callers can pass optional middlewares without branching.
func (c MiddlewareChain[Request, Response]) WithStream(middlewares ...StreamMiddleware[Request, Response]) MiddlewareChain[Request, Response] {
	c = c.Clone()
	c.stream = compactStreamMiddlewares(middlewares)
	return c
}

// CallMiddlewares returns a defensive copy of the call-side chain.
func (c MiddlewareChain[Request, Response]) CallMiddlewares() []CallMiddleware[Request, Response] {
	return slices.Clone(c.call)
}

// StreamMiddlewares returns a defensive copy of the stream-side chain.
func (c MiddlewareChain[Request, Response]) StreamMiddlewares() []StreamMiddleware[Request, Response] {
	return slices.Clone(c.stream)
}

// Clone returns a shallow copy with independent middleware slices. The
// middleware functions themselves are shared (they are values, not owned
// state).
func (c MiddlewareChain[Request, Response]) Clone() MiddlewareChain[Request, Response] {
	return MiddlewareChain[Request, Response]{
		call:   slices.Clone(c.call),
		stream: slices.Clone(c.stream),
	}
}

func compactCallMiddlewares[Request any, Response any](middlewares []CallMiddleware[Request, Response]) []CallMiddleware[Request, Response] {
	out := make([]CallMiddleware[Request, Response], 0, len(middlewares))
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		out = append(out, mw)
	}
	return out
}

func compactStreamMiddlewares[Request any, Response any](middlewares []StreamMiddleware[Request, Response]) []StreamMiddleware[Request, Response] {
	out := make([]StreamMiddleware[Request, Response], 0, len(middlewares))
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		out = append(out, mw)
	}
	return out
}
