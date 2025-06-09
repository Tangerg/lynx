package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/response"
)

type Caller struct {
	options     *Options
	middleWares *Middlewares
}

func NewCaller(options *Options) (*Caller, error) {
	if options == nil {
		return nil, errors.New("options is required")
	}

	middleWares := options.middlewares
	if middleWares == nil {
		middleWares = NewMiddlewares()
	}

	return &Caller{
		options:     options,
		middleWares: middleWares.Clone(),
	}, nil
}

func (c *Caller) Text(ctx context.Context) (string, error) {
	resp, err := c.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Text(), nil
}

func (c *Caller) ChatResponse(ctx context.Context) (*response.ChatResponse, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, err
	}
	return resp.ChatResponse(), nil
}

func (c *Caller) Response(ctx context.Context) (*Response, error) {
	request, err := NewRequest(ctx, c.options)
	if err != nil {
		return nil, err
	}
	return c.Execute(request)
}

func (c *Caller) Execute(ctx *Request) (*Response, error) {
	invoker, err := newModelInvoker(ctx.chatModel)
	if err != nil {
		return nil, err
	}
	middleWares := c.middleWares
	if middleWares == nil {
		middleWares = NewMiddlewares()
	}
	callHandler := middleWares.makeCallHandler(invoker)
	return callHandler.Call(ctx)
}
