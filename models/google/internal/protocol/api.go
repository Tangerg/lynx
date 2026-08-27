package protocol

import (
	"cmp"
	"context"
	"errors"
	"iter"
	"net/http"

	"github.com/go-resty/resty/v2"
	"google.golang.org/genai"
)

type apiConfig struct {
	APIKey string

	// Backend selects the genai backend. Zero value falls back to
	// [genai.BackendGeminiAPI] — the public Gemini API. Set to
	// [genai.BackendVertexAI] for GCP-hosted enterprise deployments;
	// Project and Location become required in that mode and APIKey
	// is ignored in favor of the supplied [genai.ClientConfig.Credentials]
	// (or ADC).
	Backend genai.Backend

	// Project is the GCP project id, required when Backend ==
	// BackendVertexAI. Ignored otherwise.
	Project string

	// Location is the GCP region (e.g. "us-central1"), required when
	// Backend == BackendVertexAI. Ignored otherwise.
	Location string

	// BaseURL overrides the genai client endpoint. Optional —
	// production users should leave it empty (the SDK picks the right
	// host per Backend). Useful for mock servers / corporate proxies.
	BaseURL string

	// HTTPClient is shared by the official Gen AI SDK and the Interactions
	// transport. Optional.
	HTTPClient *http.Client
}

func (a apiConfig) validate() error {
	// Vertex AI authenticates via ADC / service account, not API key;
	// every other backend requires the typed APIKey.
	if a.Backend != genai.BackendVertexAI && a.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	return nil
}

type api struct {
	client           *genai.Client
	interactionsHTTP *resty.Client
}

func newAPI(config apiConfig) (*api, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	clientCfg := &genai.ClientConfig{
		Backend:    config.Backend,
		HTTPClient: config.HTTPClient,
	}
	if config.Backend == 0 {
		clientCfg.Backend = genai.BackendGeminiAPI
	}
	if config.APIKey != "" {
		clientCfg.APIKey = config.APIKey
	}
	if config.Project != "" {
		clientCfg.Project = config.Project
	}
	if config.Location != "" {
		clientCfg.Location = config.Location
	}
	if config.BaseURL != "" {
		clientCfg.HTTPOptions.BaseURL = config.BaseURL
	}

	client, err := genai.NewClient(context.Background(), clientCfg)
	if err != nil {
		return nil, err
	}

	interactionsHTTP := resty.New()
	if config.HTTPClient != nil {
		interactionsHTTP = resty.NewWithClient(config.HTTPClient)
	}
	interactionsHTTP.
		SetBaseURL(cmp.Or(config.BaseURL, DefaultBaseURL)).
		SetHeader("x-goog-api-key", config.APIKey).
		SetHeader("Content-Type", "application/json")

	return &api{client: client, interactionsHTTP: interactionsHTTP}, nil
}

func (a *api) chatCompletion(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return a.wrapResult(a.client.Models.GenerateContent(ctx, modelName, contents, config))
}

func (a *api) chatCompletionStream(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	return a.wrapSequence(a.client.Models.GenerateContentStream(ctx, modelName, contents, config))
}

func (a *api) embedding(ctx context.Context, modelName string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
	return a.wrapResult(a.client.Models.EmbedContent(ctx, modelName, contents, config))
}

func (a *api) countTokens(ctx context.Context, modelName string, contents []*genai.Content, config *genai.CountTokensConfig) (*genai.CountTokensResponse, error) {
	return a.wrapResult(a.client.Models.CountTokens(ctx, modelName, contents, config))
}
