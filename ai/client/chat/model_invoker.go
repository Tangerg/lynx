package chat

import (
	"errors"
	"github.com/Tangerg/lynx/ai/model/chat/request"
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/model/chat/model"
	"github.com/Tangerg/lynx/ai/model/chat/response"
	"github.com/Tangerg/lynx/pkg/stream"
)

var (
	_ CallHandler   = (*modelInvoker)(nil)
	_ StreamHandler = (*modelInvoker)(nil)
)

type modelInvoker struct {
	chatModel model.ChatModel
}

func newModelInvoker(chatModel model.ChatModel) (*modelInvoker, error) {
	if chatModel == nil {
		return nil, errors.New("chatModel is required")
	}
	return &modelInvoker{
		chatModel: chatModel,
	}, nil
}

func (i *modelInvoker) augmentLastUserMessageOutput(request *Request) *request.ChatRequest {
	outputFormat, ok := request.Get(OutputFormat.String())
	if ok {
		request.ChatRequest().AugmentLastUserMessageText(cast.ToString(outputFormat))
	}
	return request.ChatRequest()
}

func (i *modelInvoker) Call(request *Request) (*Response, error) {
	chatResponse, err := i.chatModel.Call(
		request.Context(),
		i.augmentLastUserMessageOutput(request),
	)
	if err != nil {
		return nil, err
	}
	return NewResponse(chatResponse), nil
}

// Stream  not support structured ouptput now
func (i *modelInvoker) Stream(request *Request) (stream.Reader[*Response], error) {
	reader, err := i.chatModel.Stream(
		request.Context(),
		request.ChatRequest(),
	)
	if err != nil {
		return nil, err
	}
	return stream.Map(reader, func(t *response.ChatResponse) *Response {
		return NewResponse(t)
	}), nil
}
