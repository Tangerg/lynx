package chat

import (
	"slices"
	"sync"

	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/stream"
)

type CallHandler interface {
	Call(request *Request) (*Response, error)
}

type CallHandlerFunc func(*Request) (*Response, error)

func (c CallHandlerFunc) Call(request *Request) (*Response, error) {
	return c(request)
}

type CallMiddleware func(CallHandler) CallHandler

type StreamHandler interface {
	Stream(request *Request) (stream.Reader[result.Result[*Response]], error)
}

type StreamHandlerFunc func(request *Request) (stream.Reader[result.Result[*Response]], error)

func (c StreamHandlerFunc) Stream(request *Request) (stream.Reader[result.Result[*Response]], error) {
	return c(request)
}

type StreamMiddleware func(StreamHandler) StreamHandler

type Middlewares struct {
	mu                sync.Mutex
	callMiddlewares   []CallMiddleware
	streamMiddlewares []StreamMiddleware
}

func NewMiddlewares() *Middlewares {
	return &Middlewares{}
}

func (m *Middlewares) makeCallHandler(endpoint CallHandler) CallHandler {
	m.mu.Lock()
	defer m.mu.Unlock()

	handler := endpoint
	for i := len(m.callMiddlewares) - 1; i >= 0; i-- {
		handler = m.callMiddlewares[i](handler)
	}
	return handler
}

func (m *Middlewares) makeStreamHandler(endpoint StreamHandler) StreamHandler {
	m.mu.Lock()
	defer m.mu.Unlock()

	handler := endpoint
	for i := len(m.streamMiddlewares) - 1; i >= 0; i-- {
		handler = m.streamMiddlewares[i](handler)
	}
	return handler
}

func (m *Middlewares) UseCall(callMiddlewares ...CallMiddleware) *Middlewares {
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

func (m *Middlewares) UseStream(streamMiddlewares ...StreamMiddleware) *Middlewares {
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

func (m *Middlewares) Use(middlewares ...any) *Middlewares {
	if len(middlewares) == 0 {
		return m
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, middleware := range middlewares {
		if middleware == nil {
			continue
		}
		callMiddleware, ok := middleware.(CallMiddleware)
		if ok {
			m.callMiddlewares = append(m.callMiddlewares, callMiddleware)
		}
		streamMiddleware, ok := middleware.(StreamMiddleware)
		if ok {
			m.streamMiddlewares = append(m.streamMiddlewares, streamMiddleware)
		}
	}
	return m
}

func (m *Middlewares) Clone() *Middlewares {
	m.mu.Lock()
	defer m.mu.Unlock()

	return &Middlewares{
		callMiddlewares:   slices.Clone(m.callMiddlewares),
		streamMiddlewares: slices.Clone(m.streamMiddlewares),
	}
}
