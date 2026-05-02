package model

import (
	"slices"
	"sync"
)

// CallMiddleware wraps a [CallHandler] with cross-cutting behavior such as
// logging, retry, caching, or authentication. The signature mirrors common
// HTTP middleware patterns: take next, return a new handler that delegates.
//
// Example:
//
//	func loggingMW[Req, Resp any](next CallHandler[Req, Resp]) CallHandler[Req, Resp] {
//	    return CallHandlerFunc[Req, Resp](func(ctx context.Context, req Req) (Resp, error) {
//	        log.Printf("call begin: %T", req)
//	        resp, err := next.Call(ctx, req)
//	        log.Printf("call end: err=%v", err)
//	        return resp, err
//	    })
//	}
type CallMiddleware[Request any, Response any] func(next CallHandler[Request, Response]) CallHandler[Request, Response]

// StreamMiddleware wraps a [StreamHandler] with cross-cutting behavior.
// Behaves like [CallMiddleware] but operates over the streaming
// [iter.Seq2] result so it can intercept individual chunks.
type StreamMiddleware[Request any, Response any] func(next StreamHandler[Request, Response]) StreamHandler[Request, Response]

// MiddlewareManager keeps independent ordered chains of [CallMiddleware] and
// [StreamMiddleware] and applies them when building handlers. The four type
// parameters allow the call and stream paths to use different request /
// response types — e.g. some providers stream chunks of a different type
// than the final call response.
//
// Example:
//
//	mm := NewMiddlewareManager[*Req, *Resp, *Req, *Chunk]()
//	mm.UseCallMiddlewares(loggingMW, retryMW)
//	wrapped := mm.BuildCallHandler(rawHandler)
type MiddlewareManager[CallRequest any, CallResponse any, StreamRequest any, StreamResponse any] struct {
	mu                sync.RWMutex
	callMiddlewares   []CallMiddleware[CallRequest, CallResponse]
	streamMiddlewares []StreamMiddleware[StreamRequest, StreamResponse]
}

// NewMiddlewareManager returns an empty manager. Type inference rarely
// works here, so call sites typically spell the four type parameters out.
func NewMiddlewareManager[CallRequest any, CallResponse any, StreamRequest any, StreamResponse any]() *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	return &MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]{}
}

// BuildCallHandler wraps endpoint with the registered call middleware
// chain and returns the composed handler. Middlewares run outer-first (the
// first registered is the outermost layer); on a cold cache call sequence
// for [mw1, mw2, mw3] the runtime path is mw1 → mw2 → mw3 → endpoint.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) BuildCallHandler(endpoint CallHandler[CallRequest, CallResponse]) CallHandler[CallRequest, CallResponse] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wrapped := endpoint
	for i := len(m.callMiddlewares) - 1; i >= 0; i-- {
		wrapped = m.callMiddlewares[i](wrapped)
	}
	return wrapped
}

// BuildStreamHandler wraps endpoint with the registered stream middleware
// chain. Composition order matches [MiddlewareManager.BuildCallHandler].
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) BuildStreamHandler(endpoint StreamHandler[StreamRequest, StreamResponse]) StreamHandler[StreamRequest, StreamResponse] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wrapped := endpoint
	for i := len(m.streamMiddlewares) - 1; i >= 0; i-- {
		wrapped = m.streamMiddlewares[i](wrapped)
	}
	return wrapped
}

// UseCallMiddlewares appends call-side middlewares to the chain in the
// order given. nil entries are silently dropped so callers can safely
// pass the result of optional builders. Returns the manager for chaining.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) UseCallMiddlewares(middlewares ...CallMiddleware[CallRequest, CallResponse]) *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	if len(middlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		m.callMiddlewares = append(m.callMiddlewares, mw)
	}
	return m
}

// UseStreamMiddlewares appends stream-side middlewares to the chain.
// Behavior mirrors [MiddlewareManager.UseCallMiddlewares].
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) UseStreamMiddlewares(middlewares ...StreamMiddleware[StreamRequest, StreamResponse]) *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	if len(middlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		m.streamMiddlewares = append(m.streamMiddlewares, mw)
	}
	return m
}

// UseMiddlewares accepts mixed call- and stream-middlewares and routes
// each to the matching chain via type assertion. Values that match neither
// type — or that match both, e.g. a single function returned from
// [tool.NewToolMiddleware] — are appended to every matching chain.
//
// The any-typed parameter is the price for "register everything in one
// call"; type-mismatches are silently ignored, so callers that want
// strict registration should use [MiddlewareManager.UseCallMiddlewares] or
// [MiddlewareManager.UseStreamMiddlewares] directly.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) UseMiddlewares(middlewares ...any) *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	if len(middlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		if callMW, ok := mw.(CallMiddleware[CallRequest, CallResponse]); ok {
			m.callMiddlewares = append(m.callMiddlewares, callMW)
		}
		if streamMW, ok := mw.(StreamMiddleware[StreamRequest, StreamResponse]); ok {
			m.streamMiddlewares = append(m.streamMiddlewares, streamMW)
		}
	}
	return m
}

// Clone returns a shallow copy with independent middleware slices.
// The middleware functions themselves are shared (they are values, not
// owned state). Returns nil when the receiver is nil so callers can chain
// safely on optional managers.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) Clone() *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return &MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]{
		callMiddlewares:   slices.Clone(m.callMiddlewares),
		streamMiddlewares: slices.Clone(m.streamMiddlewares),
	}
}

// CallMiddlewareManager is the call-only specialization of
// [MiddlewareManager]. Modalities that do not stream (embedding, image,
// moderation, transcription) use this to keep their public API focused.
type CallMiddlewareManager[CallRequest any, CallResponse any] struct {
	inner *MiddlewareManager[CallRequest, CallResponse, any, any]
}

// NewCallMiddlewareManager returns an empty call-only manager.
func NewCallMiddlewareManager[CallRequest any, CallResponse any]() *CallMiddlewareManager[CallRequest, CallResponse] {
	return &CallMiddlewareManager[CallRequest, CallResponse]{
		inner: NewMiddlewareManager[CallRequest, CallResponse, any, any](),
	}
}

// UseMiddlewares appends call middlewares to the chain in registration
// order. Returns the manager for chaining.
func (c *CallMiddlewareManager[CallRequest, CallResponse]) UseMiddlewares(middlewares ...CallMiddleware[CallRequest, CallResponse]) *CallMiddlewareManager[CallRequest, CallResponse] {
	c.inner.UseCallMiddlewares(middlewares...)
	return c
}

// BuildHandler wraps endpoint with the registered chain. Composition
// order matches [MiddlewareManager.BuildCallHandler].
func (c *CallMiddlewareManager[CallRequest, CallResponse]) BuildHandler(endpoint CallHandler[CallRequest, CallResponse]) CallHandler[CallRequest, CallResponse] {
	return c.inner.BuildCallHandler(endpoint)
}

// Clone returns a shallow copy with an independent chain. Returns nil on
// a nil receiver.
func (c *CallMiddlewareManager[CallRequest, CallResponse]) Clone() *CallMiddlewareManager[CallRequest, CallResponse] {
	if c == nil {
		return nil
	}
	return &CallMiddlewareManager[CallRequest, CallResponse]{
		inner: c.inner.Clone(),
	}
}

// StreamMiddlewareManager is the stream-only specialization of
// [MiddlewareManager], the mirror of [CallMiddlewareManager].
type StreamMiddlewareManager[StreamRequest any, StreamResponse any] struct {
	inner *MiddlewareManager[any, any, StreamRequest, StreamResponse]
}

// NewStreamMiddlewareManager returns an empty stream-only manager.
func NewStreamMiddlewareManager[StreamRequest any, StreamResponse any]() *StreamMiddlewareManager[StreamRequest, StreamResponse] {
	return &StreamMiddlewareManager[StreamRequest, StreamResponse]{
		inner: NewMiddlewareManager[any, any, StreamRequest, StreamResponse](),
	}
}

// UseMiddlewares appends stream middlewares to the chain in registration
// order. Returns the manager for chaining.
func (s *StreamMiddlewareManager[StreamRequest, StreamResponse]) UseMiddlewares(middlewares ...StreamMiddleware[StreamRequest, StreamResponse]) *StreamMiddlewareManager[StreamRequest, StreamResponse] {
	s.inner.UseStreamMiddlewares(middlewares...)
	return s
}

// BuildHandler wraps endpoint with the registered chain. Composition
// order matches [MiddlewareManager.BuildStreamHandler].
func (s *StreamMiddlewareManager[StreamRequest, StreamResponse]) BuildHandler(endpoint StreamHandler[StreamRequest, StreamResponse]) StreamHandler[StreamRequest, StreamResponse] {
	return s.inner.BuildStreamHandler(endpoint)
}

// Clone returns a shallow copy with an independent chain. Returns nil on
// a nil receiver.
func (s *StreamMiddlewareManager[StreamRequest, StreamResponse]) Clone() *StreamMiddlewareManager[StreamRequest, StreamResponse] {
	if s == nil {
		return nil
	}
	return &StreamMiddlewareManager[StreamRequest, StreamResponse]{
		inner: s.inner.Clone(),
	}
}
