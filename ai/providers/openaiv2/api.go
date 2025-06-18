package openaiv2

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/Tangerg/lynx/ai/model/model"
)

type Api struct {
	apiKey model.ApiKey
	client *openai.Client
}

func NewApi(apiKey model.ApiKey) *Api {
	client := openai.NewClient(option.WithAPIKey(apiKey.Get()))
	return &Api{
		apiKey: apiKey,
		client: &client,
	}
}

func (o *Api) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return o.client.Chat.Completions.New(ctx, *req)
}

func (o *Api) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return o.client.Chat.Completions.NewStreaming(ctx, *req)
}
