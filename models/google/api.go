package google

import (
	"context"
	"errors"
	"iter"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model"
)

type ApiConfig struct {
	ApiKey model.ApiKey
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("google: config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("google: api key is required")
	}
	return nil
}

type Api struct {
	client *genai.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  cfg.ApiKey.Get(),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	return &Api{client: client}, nil
}

func (a *Api) ChatCompletion(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return a.client.Models.GenerateContent(ctx, modelName, contents, config)
}

func (a *Api) ChatCompletionStream(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	return a.client.Models.GenerateContentStream(ctx, modelName, contents, config)
}
