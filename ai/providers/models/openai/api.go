package openai

import (
	"context"
	"errors"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/Tangerg/lynx/ai/model/model"
)

type Api struct {
	apiKey model.ApiKey
	client *openai.Client
}

func NewApi(apiKey model.ApiKey, opts ...option.RequestOption) (*Api, error) {
	if apiKey == nil {
		return nil, errors.New("apiKey is required")
	}

	options := append(opts, option.WithAPIKey(apiKey.Get()))
	client := openai.NewClient(options...)

	return &Api{
		apiKey: apiKey,
		client: &client,
	}, nil
}

func (a *Api) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if req == nil {
		return nil, errors.New("invalid parameter, ChatCompletionNewParams is required")
	}

	return a.client.Chat.Completions.New(ctx, *req)
}

func (a *Api) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if req == nil {
		return nil, errors.New("invalid parameter, ChatCompletionNewParams is required")
	}

	return a.client.Chat.Completions.NewStreaming(ctx, *req), nil
}
