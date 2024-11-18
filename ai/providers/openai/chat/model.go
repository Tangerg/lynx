package chat

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/providers/openai/api"
	"io"
)

var _ model.ChatModel[*OpenAIChatRequestOptions, *OpenAIChatResultMetadata] = (*OpenAIChatModel)(nil)

type OpenAIChatModel struct {
	defaultOptions *OpenAIChatRequestOptions
	openAIApi      *api.OpenAIApi
	helper         *helper
}

func (o *OpenAIChatModel) Call(ctx context.Context, req *OpenAIChatRequest) (*OpenAIChatResponse, error) {
	creq := o.helper.makeApiChatCompletionRequest(req, false)

	cres, err := o.openAIApi.CreateChatCompletion(ctx, creq)
	if err != nil {
		return nil, err
	}

	resp := o.helper.makeOpenAICallChatResponse(&cres)

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
	creq := o.helper.makeApiChatCompletionRequest(req, true)

	stream, err := o.openAIApi.CreateChatCompletionStream(ctx, creq)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stream.Close()
	}()

	var (
		openAIChatResponse *OpenAIChatResponse
		streamResponseFunc = req.Options().StreamResponseFunc()
		aggregator         = newAggregator()
		err1               error
	)

	for {
		chunk, err2 := stream.Recv()
		if err2 != nil {
			if !errors.Is(err2, io.EOF) {
				err1 = err2
			}
			break
		}

		aggregator.addChunk(&chunk)

		openAIChatResponse = o.helper.makeOpenAIChatResponseByStreamChunk(&chunk)
		if streamResponseFunc != nil {
			err1 = streamResponseFunc(ctx, openAIChatResponse)
			if err1 != nil {
				break
			}
		}
	}

	if err1 != nil {
		return nil, err1
	}

	cres := aggregator.aggregate()
	cres.SetHeader(stream.Header())

	resp := o.helper.makeOpenAICallChatResponse(cres)

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

	return o.Stream(ctx, newReq)
}
