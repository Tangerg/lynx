package api

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/Tangerg/lynx/ai/model/model"
)

type OpenAIApi struct {
	apiKey model.ApiKey
	client *openai.Client
}

func NewOpenAIApi(apiKey model.ApiKey) *OpenAIApi {
	client := openai.NewClient(option.WithAPIKey(apiKey.Get()))
	return &OpenAIApi{
		apiKey: apiKey,
		client: &client,
	}
}

func (o *OpenAIApi) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return o.client.Chat.Completions.New(ctx, *req)
}

func (o *OpenAIApi) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return o.client.Chat.Completions.NewStreaming(ctx, *req)
}
