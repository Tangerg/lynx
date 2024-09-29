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

func (c *OpenAIApi) CreateCompletion(ctx context.Context, request *openai.CompletionRequest) (openai.CompletionResponse, error) {
	return c.client.CreateCompletion(ctx, *request)
}

func (c *OpenAIApi) CreateCompletionStream(ctx context.Context, request *openai.CompletionRequest) (*openai.CompletionStream, error) {
	return c.client.CreateCompletionStream(ctx, *request)
}
