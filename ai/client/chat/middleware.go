package chat

import (
	"slices"

	"github.com/Tangerg/lynx/pkg/stream"
)

type CallHandler interface {
	Call(request *Request) (*Response, error)
}

type CallMiddleware func(CallHandler) CallHandler

type CallHandlerFunc func(*Request) (*Response, error)

func (c CallHandlerFunc) Call(request *Request) (*Response, error) {
	return c(request)
}

type StreamHandler interface {
	Stream(request *Request) (stream.Reader[*Response], error)
}

type StreamMiddleware func(StreamHandler) StreamHandler

type StreamHandlerFunc func(*Request) (stream.Reader[*Response], error)

func (c StreamHandlerFunc) Stream(request *Request) (stream.Reader[*Response], error) {
	return c(request)
}

type Middlewares struct {
	callMiddlewares   []CallMiddleware
	streamMiddlewares []StreamMiddleware
}

func NewMiddlewares() *Middlewares {
	return &Middlewares{}
}

func (m *Middlewares) makeCallHandler(endpoint CallHandler) CallHandler {
	handler := endpoint
	for i := len(m.callMiddlewares) - 1; i >= 0; i-- {
		handler = m.callMiddlewares[i](handler)
	}
	return handler
}

func (m *Middlewares) makeStreamHandler(endpoint StreamHandler) StreamHandler {
	handler := endpoint
	for i := len(m.streamMiddlewares) - 1; i >= 0; i-- {
		handler = m.streamMiddlewares[i](handler)
	}
	return handler
}

func (m *Middlewares) Add(middlewares ...any) {
	for _, middleware := range middlewares {
		callMiddleware, ok := middleware.(CallMiddleware)
		if ok {
			m.callMiddlewares = append(m.callMiddlewares, callMiddleware)
		}
		streamMiddleware, ok := middleware.(StreamMiddleware)
		if ok {
			m.streamMiddlewares = append(m.streamMiddlewares, streamMiddleware)
		}
	}
}

func (m *Middlewares) Clone() *Middlewares {
	return &Middlewares{
		callMiddlewares:   slices.Clone(m.callMiddlewares),
		streamMiddlewares: slices.Clone(m.streamMiddlewares),
	}
}
