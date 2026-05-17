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

	// Backend selects the genai backend. Zero value falls back to
	// [genai.BackendGeminiAPI] — the public Gemini API. Set to
	// [genai.BackendVertexAI] for GCP-hosted enterprise deployments;
	// Project and Location become required in that mode and ApiKey
	// is ignored in favor of the supplied [genai.ClientConfig.Credentials]
	// (or ADC).
	Backend genai.Backend

	// Project is the GCP project id, required when Backend ==
	// BackendVertexAI. Ignored otherwise.
	Project string

	// Location is the GCP region (e.g. "us-central1"), required when
	// Backend == BackendVertexAI. Ignored otherwise.
	Location string
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("google: config must not be nil")
	}
	// Vertex AI authenticates via ADC / service account, not API key;
	// every other backend requires the typed ApiKey.
	if c.Backend != genai.BackendVertexAI && c.ApiKey == nil {
		return errors.New("google: ApiKey is required")
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

	clientCfg := &genai.ClientConfig{Backend: cfg.Backend}
	if cfg.Backend == 0 {
		clientCfg.Backend = genai.BackendGeminiAPI
	}
	if cfg.ApiKey != nil {
		clientCfg.APIKey = cfg.ApiKey.Get()
	}
	if cfg.Project != "" {
		clientCfg.Project = cfg.Project
	}
	if cfg.Location != "" {
		clientCfg.Location = cfg.Location
	}

	client, err := genai.NewClient(context.Background(), clientCfg)
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

func (a *Api) Embedding(ctx context.Context, modelName string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
	return a.client.Models.EmbedContent(ctx, modelName, contents, config)
}

func (a *Api) Image(ctx context.Context, modelName string, prompt string, config *genai.GenerateImagesConfig) (*genai.GenerateImagesResponse, error) {
	return a.client.Models.GenerateImages(ctx, modelName, prompt, config)
}

func (a *Api) CountTokens(ctx context.Context, modelName string, contents []*genai.Content, config *genai.CountTokensConfig) (*genai.CountTokensResponse, error) {
	return a.client.Models.CountTokens(ctx, modelName, contents, config)
}

func (a *Api) ComputeTokens(ctx context.Context, modelName string, contents []*genai.Content, config *genai.ComputeTokensConfig) (*genai.ComputeTokensResponse, error) {
	return a.client.Models.ComputeTokens(ctx, modelName, contents, config)
}
