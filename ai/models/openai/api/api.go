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

func (c *OpenAIApi) CreateCompletion(ctx context.Context, request *CompletionRequest) (CompletionResponse, error) {
	return c.client.CreateCompletion(ctx, *request)
}

func (c *OpenAIApi) CreateCompletionStream(ctx context.Context, request *CompletionRequest) (*CompletionStream, error) {
	return c.client.CreateCompletionStream(ctx, *request)
}
