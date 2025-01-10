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

func (c *OpenAIApi) CreateChatCompletion(ctx context.Context, request *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	res, err := c.client.CreateChatCompletion(ctx, *request)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *OpenAIApi) CreateChatCompletionStream(ctx context.Context, request *openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return c.client.CreateChatCompletionStream(ctx, *request)
}

func (c *OpenAIApi) CreateEmbeddings(ctx context.Context, request *openai.EmbeddingRequestStrings) (*openai.EmbeddingResponse, error) {
	res, err := c.client.CreateEmbeddings(ctx, *request)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *OpenAIApi) CreateImage(ctx context.Context, request *openai.ImageRequest) (*openai.ImageResponse, error) {
	res, err := c.client.CreateImage(ctx, *request)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *OpenAIApi) CreateTranscription(ctx context.Context, request *openai.AudioRequest) (*openai.AudioResponse, error) {
	res, err := c.client.CreateTranscription(ctx, *request)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
