package model

import (
	"slices"
	"sync"
)

// CallMiddleware defines a function type for implementing middleware that wraps
// CallHandler instances. Middleware provides a way to add cross-cutting concerns
// such as logging, authentication, rate limiting, caching, and error handling
// to AI model calls without modifying the core handler logic.
type CallMiddleware[Request any, Response any] func(handler CallHandler[Request, Response]) CallHandler[Request, Response]

// StreamMiddleware defines a function type for implementing middleware that wraps
// StreamHandler instances. Similar to CallMiddleware, it provides a way to add
// cross-cutting concerns to streaming AI model calls while maintaining the
// streaming nature of the responses.
type StreamMiddleware[Request any, Response any] func(handler StreamHandler[Request, Response]) StreamHandler[Request, Response]

// MiddlewareManager manages and applies middleware chains for both synchronous
// call handlers and streaming handlers in AI model implementations. It provides
// a centralized way to configure, organize, and apply middleware to handlers,
// ensuring consistent behavior across different AI model endpoints.
type MiddlewareManager[CallRequest any, CallResponse any, StreamRequest any, StreamResponse any] struct {
	mu                sync.RWMutex
	callMiddlewares   []CallMiddleware[CallRequest, CallResponse]
	streamMiddlewares []StreamMiddleware[StreamRequest, StreamResponse]
}

// BuildCallHandler applies the registered call middleware chain to the provided
// CallHandler endpoint. The middleware is applied in reverse order (last added, first executed).
// This method is thread-safe and uses a read lock for consistent middleware chain application.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) BuildCallHandler(endpoint CallHandler[CallRequest, CallResponse]) CallHandler[CallRequest, CallResponse] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	currentHandler := endpoint
	for i := len(m.callMiddlewares) - 1; i >= 0; i-- {
		currentHandler = m.callMiddlewares[i](currentHandler)
	}

	return currentHandler
}

// BuildStreamHandler applies the registered stream middleware chain to the provided
// StreamHandler endpoint. Similar to BuildCallHandler, middleware is applied in reverse order.
// This method ensures streaming-specific middleware is properly applied to the streaming handler.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) BuildStreamHandler(endpoint StreamHandler[StreamRequest, StreamResponse]) StreamHandler[StreamRequest, StreamResponse] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	currentHandler := endpoint
	for i := len(m.streamMiddlewares) - 1; i >= 0; i-- {
		currentHandler = m.streamMiddlewares[i](currentHandler)
	}

	return currentHandler
}

// UseCallMiddlewares registers one or more CallMiddleware instances to be applied
// to CallHandler endpoints. The middleware will be applied in registration order.
// Returns the MiddlewareManager instance for method chaining.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) UseCallMiddlewares(callMiddlewares ...CallMiddleware[CallRequest, CallResponse]) *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	if len(callMiddlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, callMiddleware := range callMiddlewares {
		if callMiddleware == nil {
			continue
		}

		m.callMiddlewares = append(m.callMiddlewares, callMiddleware)
	}

	return m
}

// UseStreamMiddlewares registers one or more StreamMiddleware instances to be
// applied to StreamHandler endpoints. Maintains the same fluent interface pattern.
// Thread-safe registration with middleware execution order preservation.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) UseStreamMiddlewares(streamMiddlewares ...StreamMiddleware[StreamRequest, StreamResponse]) *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	if len(streamMiddlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, streamMiddleware := range streamMiddlewares {
		if streamMiddleware == nil {
			continue
		}

		m.streamMiddlewares = append(m.streamMiddlewares, streamMiddleware)
	}

	return m
}

// UseMiddlewares provides a convenient way to register multiple middleware of
// different types in a single call. Automatically determines middleware type
// using type assertions and registers them in the appropriate chain.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) UseMiddlewares(middlewares ...any) *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	if len(middlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, middleware := range middlewares {
		if middleware == nil {
			continue
		}

		if callMiddleware, ok := middleware.(CallMiddleware[CallRequest, CallResponse]); ok {
			m.callMiddlewares = append(m.callMiddlewares, callMiddleware)
		}

		if streamMiddleware, ok := middleware.(StreamMiddleware[StreamRequest, StreamResponse]); ok {
			m.streamMiddlewares = append(m.streamMiddlewares, streamMiddleware)
		}
	}

	return m
}

// Clone creates a deep copy of the MiddlewareManager with independent middleware
// chains. Useful for creating separate configurations that start with the same base
// middleware but may diverge over time. Thread-safe operation with no shared state.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) Clone() *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse] {
	m.mu.Lock()
	defer m.mu.Unlock()

	return &MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]{
		callMiddlewares:   slices.Clone(m.callMiddlewares),
		streamMiddlewares: slices.Clone(m.streamMiddlewares),
	}
}
