package model

import (
	"slices"
	"sync"
)

// CallMiddleware wraps a [CallHandler] with cross-cutting behavior
// (logging, retry, caching, auth). Standard HTTP-middleware shape:
// take next, return a wrapping handler that delegates.
type CallMiddleware[Request any, Response any] func(next CallHandler[Request, Response]) CallHandler[Request, Response]

// StreamMiddleware wraps a [StreamHandler] with cross-cutting behavior.
// Behaves like [CallMiddleware] but operates over the streaming
// [iter.Seq2] result so it can intercept individual chunks.
type StreamMiddleware[Request any, Response any] func(next StreamHandler[Request, Response]) StreamHandler[Request, Response]

// MiddlewareManager keeps independent ordered chains of [CallMiddleware]
// and [StreamMiddleware] for a single [Request] / [Response] pair.
// Modalities that don't stream simply never register stream middlewares;
// the stream slice stays empty.
//
// Example:
//
//	mm := NewMiddlewareManager[*Req, *Resp]()
//	mm.UseCallMiddlewares(loggingMW, retryMW)
//	wrapped := mm.BuildCallHandler(rawHandler)
type MiddlewareManager[Request any, Response any] struct {
	mu                sync.RWMutex
	callMiddlewares   []CallMiddleware[Request, Response]
	streamMiddlewares []StreamMiddleware[Request, Response]
}

// NewMiddlewareManager returns an empty manager. Type inference rarely
// works here, so call sites typically spell the two type parameters out.
func NewMiddlewareManager[Request any, Response any]() *MiddlewareManager[Request, Response] {
	return &MiddlewareManager[Request, Response]{}
}

// BuildCallHandler wraps endpoint with the registered call middleware
// chain and returns the composed handler. Middlewares run outer-first (the
// first registered is the outermost layer); on a cold cache call sequence
// for [mw1, mw2, mw3] the runtime path is mw1 → mw2 → mw3 → endpoint.
func (m *MiddlewareManager[Request, Response]) BuildCallHandler(endpoint CallHandler[Request, Response]) CallHandler[Request, Response] {
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
func (m *MiddlewareManager[Request, Response]) BuildStreamHandler(endpoint StreamHandler[Request, Response]) StreamHandler[Request, Response] {
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
func (m *MiddlewareManager[Request, Response]) UseCallMiddlewares(middlewares ...CallMiddleware[Request, Response]) *MiddlewareManager[Request, Response] {
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
func (m *MiddlewareManager[Request, Response]) UseStreamMiddlewares(middlewares ...StreamMiddleware[Request, Response]) *MiddlewareManager[Request, Response] {
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
// each to the matching chain via type assertion — so the (call, stream)
// pair returned by tool.NewMiddleware registers in one call. A value
// that matches both types lands on both chains.
//
// The any-typed parameter is the price for "register everything in one
// call"; type-mismatches are silently ignored, so callers that want
// strict registration should use [MiddlewareManager.UseCallMiddlewares] or
// [MiddlewareManager.UseStreamMiddlewares] directly.
func (m *MiddlewareManager[Request, Response]) UseMiddlewares(middlewares ...any) *MiddlewareManager[Request, Response] {
	if len(middlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		if callMW, ok := mw.(CallMiddleware[Request, Response]); ok {
			m.callMiddlewares = append(m.callMiddlewares, callMW)
		}
		if streamMW, ok := mw.(StreamMiddleware[Request, Response]); ok {
			m.streamMiddlewares = append(m.streamMiddlewares, streamMW)
		}
	}
	return m
}

// Clone returns a shallow copy with independent middleware slices.
// The middleware functions themselves are shared (they are values, not
// owned state). Returns nil when the receiver is nil so callers can chain
// safely on optional managers.
func (m *MiddlewareManager[Request, Response]) Clone() *MiddlewareManager[Request, Response] {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return &MiddlewareManager[Request, Response]{
		callMiddlewares:   slices.Clone(m.callMiddlewares),
		streamMiddlewares: slices.Clone(m.streamMiddlewares),
	}
}
