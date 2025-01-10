package chat

import (
	"context"
	"errors"
	"io"

	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/providers/openai/api"
)

var _ model.ChatModel[*OpenAIChatRequestOptions, *OpenAIChatResultMetadata] = (*OpenAIChatModel)(nil)

type OpenAIChatModel struct {
	defaultOptions *OpenAIChatRequestOptions
	openAIApi      *api.OpenAIApi
	helper         *helper
	converter      *converter
}

func NewOpenAIChatModel(openAIApi *api.OpenAIApi, defaultOptions *OpenAIChatRequestOptions) model.ChatModel[*OpenAIChatRequestOptions, *OpenAIChatResultMetadata] {
	return &OpenAIChatModel{
		openAIApi:      openAIApi,
		defaultOptions: defaultOptions,
		helper:         newHelper(),
		converter:      newConverter(),
	}
}

func (o *OpenAIChatModel) Call(ctx context.Context, req *OpenAIChatRequest) (*OpenAIChatResponse, error) {
	o.helper.RegisterFunctions(req.Options().Functions()...)
	creq := o.converter.makeApiChatCompletionRequest(req, false)

	cres, err := o.openAIApi.CreateChatCompletion(ctx, creq)
	if err != nil {
		return nil, err
	}

	resp := o.converter.makeOpenAIChatResponse(cres)

	if !o.helper.shouldHandleToolCalls(o.defaultOptions, req, resp) {
		return resp, nil
	}

	newReq, err := o.helper.handleToolCalls(ctx, req, resp)
	if err != nil {
		return resp, err
	}

	return o.Call(ctx, newReq)
}

func (o *OpenAIChatModel) Stream(ctx context.Context, req *OpenAIChatRequest, handler OpenAIChatStreamChunkHandler) (*OpenAIChatResponse, error) {
	o.helper.RegisterFunctions(req.Options().Functions()...)
	creq := o.converter.makeApiChatCompletionRequest(req, true)

	stream, err := o.openAIApi.CreateChatCompletionStream(ctx, creq)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stream.Close()
	}()

	aggregator := newAggregator()

	for {
		chunk, err1 := stream.Recv()
		if err1 != nil {
			if errors.Is(err1, io.EOF) {
				break
			}
			return nil, err1
		}

		aggregator.addChunk(&chunk)

		openAIChatResponse := o.converter.makeOpenAIChatResponseByStreamChunk(&chunk)
		if handler != nil {
			err1 = handler(ctx, openAIChatResponse)
			if err1 != nil {
				return nil, err1
			}
		}
	}

	cres := aggregator.aggregate()
	cres.SetHeader(stream.Header())

	resp := o.converter.makeOpenAIChatResponse(cres)

	if !o.helper.shouldHandleToolCalls(o.defaultOptions, req, resp) {
		return resp, nil
	}

	newReq, err := o.helper.handleToolCalls(ctx, req, resp)
	if err != nil {
		return resp, err
	}

	return o.Stream(ctx, newReq, handler)
}
