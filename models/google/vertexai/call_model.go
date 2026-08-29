package vertexai

import (
	"context"
	"errors"
)

type modelCaller[Request, Response any] interface {
	Call(context.Context, *Request) (*Response, error)
}

type callModel[Request, Response any] struct {
	model modelCaller[Request, Response]
}

func newCallModel[Request, Response any](model modelCaller[Request, Response], err error) (*callModel[Request, Response], error) {
	if err != nil {
		return nil, err
	}
	return &callModel[Request, Response]{model: model}, nil
}

func (c *callModel[Request, Response]) Call(ctx context.Context, req *Request) (*Response, error) {
	if c == nil || c.model == nil {
		return nil, errors.New("vertexai: nil model")
	}
	return c.model.Call(ctx, req)
}
