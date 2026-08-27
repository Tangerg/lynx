package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/openai/openai-go/v3"

	"github.com/Tangerg/scope/core/embedding"
)

type EmbeddingModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (e EmbeddingModelConfig) Validate() error {
	if err := validateProvider(e.Provider); err != nil {
		return fmt.Errorf("openai: Provider: %w", err)
	}
	if e.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if e.DefaultOptions.Model == "" {
		return errors.New("openai: DefaultOptions.Model is required")
	}
	if err := e.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct {
	api            *api
	provider       string
	defaultOptions embedding.Options
}

func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{
		api:            api,
		provider:       cfg.Provider,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (e *EmbeddingModel) buildAPIEmbeddingRequest(req *embedding.Request) (*openai.EmbeddingNewParams, error) {
	effectiveOptions, err := e.defaultOptions.Resolve(req.Options)
	if err != nil {
		return nil, err
	}

	fields, err := decodeRequestFields(effectiveOptions.Extensions, protocolModalityRequestExtensionKey(e.provider, "embedding"), "model", "input", "dimensions")
	if err != nil {
		return nil, err
	}
	params := &openai.EmbeddingNewParams{}
	params.SetExtraFields(fields)

	params.Model = effectiveOptions.Model
	params.Input = openai.EmbeddingNewParamsInputUnion{
		OfArrayOfStrings: req.Texts,
	}

	if effectiveOptions.Dimensions != nil {
		params.Dimensions = openai.Int(*effectiveOptions.Dimensions)
	}

	return params, nil
}

func (e *EmbeddingModel) buildEmbeddingResponse(apiResp *openai.CreateEmbeddingResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Data) == 0 {
		return nil, errors.New("openai: embeddings response has no data")
	}
	if len(apiResp.Data) != expectedResults {
		return nil, fmt.Errorf("openai: embeddings response returned %d outputs for %d inputs", len(apiResp.Data), expectedResults)
	}

	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &embedding.Usage{
			InputTokens: apiResp.Usage.PromptTokens,
		},
	}

	outputs := make([]*embedding.Output, len(apiResp.Data))
	seen := make([]bool, len(apiResp.Data))
	for _, item := range apiResp.Data {
		if item.Index < 0 || item.Index >= int64(len(outputs)) {
			return nil, fmt.Errorf("openai: embeddings response index %d is out of range", item.Index)
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("openai: embeddings response repeats index %d", item.Index)
		}
		outputMetadata := &embedding.OutputMetadata{}

		output, err := embedding.NewOutput(item.Embedding, outputMetadata)
		if err != nil {
			return nil, err
		}

		outputs[item.Index] = output
		seen[item.Index] = true
	}

	return embedding.NewResponse(outputs, meta)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := e.buildAPIEmbeddingRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := e.api.embedding(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildEmbeddingResponse(apiResp, len(req.Texts))
}
