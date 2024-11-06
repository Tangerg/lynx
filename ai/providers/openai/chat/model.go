package chat

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/providers/openai/api"
	"github.com/sashabaranov/go-openai"
	"io"
)

var _ model.ChatModel[*OpenAIChatRequestOptions, *OpenAIChatResultMetadata] = (*OpenAIChatModel)(nil)

type OpenAIChatModel struct {
	defaultOptions *OpenAIChatRequestOptions
	openAIApi      *api.OpenAIApi
	helper         *helper
}

func (o *OpenAIChatModel) Call(ctx context.Context, req *OpenAIChatRequest) (*OpenAIChatResponse, error) {
	creq := o.helper.createApiChatCompletionRequest(req, false)

	cres, err := o.openAIApi.CreateChatCompletion(ctx, creq)
	if err != nil {
		return nil, err
	}

	resp := o.helper.createOpenAICallChatResponse(&cres)

	if o.helper.IsProxyToolCalls(req.Options(), o.defaultOptions) {
		return resp, nil
	}

	if !o.helper.IsToolCallChatCompletion(
		resp,
		[]result.FinishReason{result.ToolCalls, result.Stop},
	) {
		return resp, nil
	}

	msgs, err := o.helper.HandleToolCalls(ctx, req, resp)
	if err != nil {
		return resp, err
	}

	newReq, _ := newOpenAIChatRequestBuilder().
		WithOptions(req.Options()).
		WithMessages(msgs...).
		Build()

	return o.Call(ctx, newReq)
}

func (o *OpenAIChatModel) Stream(ctx context.Context, req *OpenAIChatRequest) (*OpenAIChatResponse, error) {
	creq := o.helper.createApiChatCompletionRequest(req, true)

	stream, err := o.openAIApi.CreateChatCompletionStream(ctx, creq)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stream.Close()
	}()

	var (
		openAIChatResponse *OpenAIChatResponse
		recv               openai.ChatCompletionStreamResponse
		recvs              = make([]openai.ChatCompletionStreamResponse, 0, 64)
		streamResponseFunc = req.Options().StreamResponseFunc()
	)

	for {
		recv, err = stream.Recv()

		if err != nil {
			if err == io.EOF {
				return o.helper.merageApiChatCompletionStreamResponse(recvs)
			}
			return openAIChatResponse, err
		}

		if len(recv.Choices) == 0 {
			continue
		}

		recvs = append(recvs, recv)

		openAIChatResponse = o.helper.createOpenAIStreamChatResponse(&recv)
		if streamResponseFunc != nil {
			err = streamResponseFunc(ctx, openAIChatResponse)
			if err != nil {
				return openAIChatResponse, err
			}
		}
	}
}
