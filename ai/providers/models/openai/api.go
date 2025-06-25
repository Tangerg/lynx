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

func NewApi(apiKey model.ApiKey, opts ...option.RequestOption) *Api {
	options := make([]option.RequestOption, 0, len(opts)+1)
	options = append(options, opts...)
	options = append(options, option.WithAPIKey(apiKey.Get()))

	client := openai.NewClient(options...)
	return &Api{
		apiKey: apiKey,
		client: &client,
	}
}

func (o *Api) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if req == nil {
		return nil, errors.New("invalid parameter, ChatCompletionNewParams cannot be nil")
	}
	return o.client.Chat.Completions.New(ctx, *req)
}

func (o *Api) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if req == nil {
		return nil, errors.New("invalid parameter, ChatCompletionNewParams cannot be nil")
	}
	return o.client.Chat.Completions.NewStreaming(ctx, *req), nil
}
