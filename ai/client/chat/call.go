package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/response"
)

type Call struct {
	request     *Request
	middleWares *Middlewares
}

func NewCall(request *Request, middleWares ...*Middlewares) (*Call, error) {
	if request == nil {
		return nil, errors.New("request is required")
	}
	var md *Middlewares
	if len(middleWares) > 0 &&
		middleWares[0] != nil {
		md = middleWares[0]
	} else {
		md = NewMiddlewares()
	}

	return &Call{
		request:     request,
		middleWares: md.Clone(),
	}, nil
}

func (c *Call) Text(ctx context.Context) (string, error) {
	resp, err := c.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Text(), nil
}

func (c *Call) ChatResponse(ctx context.Context) (*response.ChatResponse, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, err
	}
	return resp.ChatResponse(), nil
}

func (c *Call) Response(ctx context.Context) (*Response, error) {
	return c.do(newContextFromRequest(ctx, c.request))
}

func (c *Call) do(ctx *Context) (*Response, error) {
	invoker := newModelInvoker(c.request.chatModel)
	callHandler := c.middleWares.makeCallHandler(invoker)
	return callHandler.Call(ctx)
}
