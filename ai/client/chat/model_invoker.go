package chat

import (
	"errors"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/stream"
)

var (
	_ CallHandler   = (*modelInvoker)(nil)
	_ StreamHandler = (*modelInvoker)(nil)
)

type modelInvoker struct {
	chatModel chat.Model
}

func newModelInvoker(chatModel chat.Model) (*modelInvoker, error) {
	if chatModel == nil {
		return nil, errors.New("chatModel is required")
	}

	return &modelInvoker{
		chatModel: chatModel,
	}, nil
}

func (i *modelInvoker) augmentLastUserMessageOutput(request *Request) *chat.Request {
	outputFormat, ok := request.Get(AttrChatOutputFormat.String())
	if !ok {
		return request.ChatRequest()
	}

	chatRequest := request.ChatRequest()
	chatRequest.AugmentLastUserMessageText(cast.ToString(outputFormat))

	return chatRequest
}

func (i *modelInvoker) Call(request *Request) (*Response, error) {
	chatRequest := i.augmentLastUserMessageOutput(request)

	chatResponse, err := i.chatModel.Call(request.Context(), chatRequest)
	if err != nil {
		return nil, err
	}

	return NewResponse(chatResponse), nil
}

// Stream not support structured output now
func (i *modelInvoker) Stream(request *Request) (stream.Reader[result.Result[*Response]], error) {
	chatRequest := request.ChatRequest()

	reader, err := i.chatModel.Stream(request.Context(), chatRequest)
	if err != nil {
		return nil, err
	}

	responseReader := stream.Map(
		reader,
		func(chatResult result.Result[*chat.Response]) result.Result[*Response] {
			return result.Map(
				chatResult,
				func(chatResponse *chat.Response) *Response {
					return NewResponse(chatResponse)
				},
			)
		},
	)

	return responseReader, nil
}
