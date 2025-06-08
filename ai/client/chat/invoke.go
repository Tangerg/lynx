package chat

import (
	"github.com/Tangerg/lynx/ai/model/chat/model"
	"github.com/Tangerg/lynx/ai/model/chat/response"
	"github.com/Tangerg/lynx/pkg/stream"
)

var (
	_ CallHandler   = (*modelInvoker)(nil)
	_ StreamHandler = (*modelInvoker)(nil)
)

type modelInvoker struct {
	model model.ChatModel
}

func newModelInvoker(model model.ChatModel) *modelInvoker {
	return &modelInvoker{
		model: model,
	}
}

func (i *modelInvoker) Call(ctx *Context) (*Response, error) {
	chatResponse, err := i.model.Call(ctx.Context(), ctx.ChatRequest())
	if err != nil {
		return nil, err
	}
	return NewResponse(chatResponse), nil
}

func (i *modelInvoker) Stream(ctx *Context) (stream.Reader[*Response], error) {
	reader, err := i.model.Stream(ctx.Context(), ctx.ChatRequest())
	if err != nil {
		return nil, err
	}
	return stream.Map(reader, func(t *response.ChatResponse) *Response {
		return NewResponse(t)
	}), nil
}
