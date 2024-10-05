package api

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

type OpenAIApi struct {
	client *openai.Client
}

func NewOpenAIApi(token string) *OpenAIApi {
	client := openai.NewClient(token)
	return &OpenAIApi{client: client}
}

func (c *OpenAIApi) CreateChatCompletion(ctx context.Context, request *openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return c.client.CreateChatCompletion(ctx, *request)
}

func (c *OpenAIApi) CreateChatCompletionStream(ctx context.Context, request *openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return c.client.CreateChatCompletionStream(ctx, *request)
}
