package model

import (
	"context"
	"iter"
	"slices"
	"sync"
)

// CallHandler provides a generic API for invoking AI models with synchronous
// request-response patterns. It abstracts the interaction with various types
// of AI models by handling the process of sending requests and receiving
// complete responses. The interface uses Go generics to accommodate different
// request and response types, enhancing flexibility and adaptability across
// different AI model implementations.
//
// CallHandler is suitable for scenarios where you need the full result before
// proceeding, such as:
//   - Single-turn conversations or Q&A sessions
//   - Batch processing of embeddings
//   - Image generation requests
//   - Classification or analysis tasks
//   - Function calling with complete responses
type CallHandler[Request any, Response any] interface {
	// Call executes a request to the AI model and returns the complete response.
	// This method is synchronous and blocks until the model generates the full
	// response or an error occurs.
	Call(ctx context.Context, req Request) (Response, error)
}

// CallHandlerFunc is a function type that implements the CallHandler interface.
// It allows regular functions to be used as CallHandler implementations,
// providing a convenient way to create handlers without defining new types.
type CallHandlerFunc[Request any, Response any] func(ctx context.Context, req Request) (Response, error)

// Call implements the CallHandler interface for CallHandlerFunc.
// It delegates to the underlying function, allowing function types
// to be used wherever a CallHandler is expected.
func (c CallHandlerFunc[Request, Response]) Call(ctx context.Context, req Request) (Response, error) {
	return c(ctx, req)
}

// StreamHandler provides a generic API for invoking AI models with streaming
// responses. It abstracts the process of sending requests and receiving responses
// incrementally, chunk by chunk. The interface uses Go generics to accommodate
// different request and response chunk types, enhancing flexibility and
// adaptability across different AI model implementations.
//
// StreamHandler is particularly useful for:
//   - Real-time chat applications where responses appear incrementally
//   - Long-form content generation where users want to see progress
//   - Large batch processing where memory efficiency is important
//   - Interactive applications requiring immediate feedback
//   - Server-sent events for AI model responses
//
// The streaming approach provides several benefits:
//   - Improved user experience with real-time feedback
//   - Memory efficiency by processing responses incrementally
//   - Better resource utilization through backpressure handling
//   - Early termination capability when response is satisfactory
type StreamHandler[Request any, Response any] interface {
	// Stream executes a request to the AI model and returns an iterator for
	// receiving response chunks incrementally. This allows real-time processing
	// of the model's output as it becomes available.
	Stream(ctx context.Context, req Request) iter.Seq2[Response, error]
}

// StreamHandlerFunc is a function type that implements the StreamHandler interface.
// It allows regular functions to be used as StreamHandler implementations,
// providing a convenient way to create streaming handlers without defining new types.
type StreamHandlerFunc[Request any, Response any] func(ctx context.Context, req Request) iter.Seq2[Response, error]

// Stream implements the StreamHandler interface for StreamHandlerFunc.
// It delegates to the underlying function, allowing function types
// to be used wherever a StreamHandler is expected.
func (s StreamHandlerFunc[Request, Response]) Stream(ctx context.Context, req Request) iter.Seq2[Response, error] {
	return s(ctx, req)
}

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

// MakeCallHandler applies the registered call middleware chain to the provided
// CallHandler endpoint. The middleware is applied in reverse order (last added, first executed).
// This method is thread-safe and uses a read lock for consistent middleware chain application.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) MakeCallHandler(endpoint CallHandler[CallRequest, CallResponse]) CallHandler[CallRequest, CallResponse] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	currentHandler := endpoint
	for i := len(m.callMiddlewares) - 1; i >= 0; i-- {
		currentHandler = m.callMiddlewares[i](currentHandler)
	}

	return currentHandler
}

// MakeStreamHandler applies the registered stream middleware chain to the provided
// StreamHandler endpoint. Similar to MakeCallHandler, middleware is applied in reverse order.
// This method ensures streaming-specific middleware is properly applied to the streaming handler.
func (m *MiddlewareManager[CallRequest, CallResponse, StreamRequest, StreamResponse]) MakeStreamHandler(endpoint StreamHandler[StreamRequest, StreamResponse]) StreamHandler[StreamRequest, StreamResponse] {
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
