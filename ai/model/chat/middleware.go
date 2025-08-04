package chat

import (
	"context"
	"iter"
	"slices"
	"sync"
)

type CallHandler interface {
	Call(ctx context.Context, req *Request) (*Response, error)
}
type CallHandlerFunc func(ctx context.Context, req *Request) (*Response, error)

func (f CallHandlerFunc) Call(ctx context.Context, req *Request) (*Response, error) {
	return f(ctx, req)
}

type StreamHandler interface {
	Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error]
}

type StreamHandlerFunc func(ctx context.Context, req *Request) iter.Seq2[*Response, error]

func (f StreamHandlerFunc) Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return f(ctx, req)
}

type CallMiddleware func(CallHandler) CallHandler
type StreamMiddleware func(StreamHandler) StreamHandler

type MiddlewareManager struct {
	mu                sync.Mutex
	callMiddlewares   []CallMiddleware
	streamMiddlewares []StreamMiddleware
}

func NewMiddlewareManager() *MiddlewareManager {
	return &MiddlewareManager{}
}

func (m *MiddlewareManager) makeCallHandler(endpoint CallHandler) CallHandler {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentHandler := endpoint
	for i := len(m.callMiddlewares) - 1; i >= 0; i-- {
		currentHandler = m.callMiddlewares[i](currentHandler)
	}

	return currentHandler
}

func (m *MiddlewareManager) makeStreamHandler(endpoint StreamHandler) StreamHandler {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentHandler := endpoint
	for i := len(m.streamMiddlewares) - 1; i >= 0; i-- {
		currentHandler = m.streamMiddlewares[i](currentHandler)
	}

	return currentHandler
}

func (m *MiddlewareManager) UseCallMiddlewares(callMiddlewares ...CallMiddleware) *MiddlewareManager {
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

func (m *MiddlewareManager) UseStreamMiddlewares(streamMiddlewares ...StreamMiddleware) *MiddlewareManager {
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

func (m *MiddlewareManager) UseMiddlewares(middlewares ...any) *MiddlewareManager {
	if len(middlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, middleware := range middlewares {
		if middleware == nil {
			continue
		}

		if callMiddleware, ok := middleware.(CallMiddleware); ok {
			m.callMiddlewares = append(m.callMiddlewares, callMiddleware)
		}

		if streamMiddleware, ok := middleware.(StreamMiddleware); ok {
			m.streamMiddlewares = append(m.streamMiddlewares, streamMiddleware)
		}
	}

	return m
}

func (m *MiddlewareManager) Clone() *MiddlewareManager {
	m.mu.Lock()
	defer m.mu.Unlock()

	return &MiddlewareManager{
		callMiddlewares:   slices.Clone(m.callMiddlewares),
		streamMiddlewares: slices.Clone(m.streamMiddlewares),
	}
}
