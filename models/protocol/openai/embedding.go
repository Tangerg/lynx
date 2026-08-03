package openai

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
)

type EmbeddingModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions embedding.Options
	RequestOptions []option.RequestOption
}

func (c EmbeddingModelConfig) Validate() error {
	if err := validateProvider(c.Provider); err != nil {
		return fmt.Errorf("openai: Provider: %w", err)
	}
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("openai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct {
	api            *API
	provider       string
	defaultOptions embedding.Options
}

func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
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
	mergedOpts, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	paramsValue, _, err := metadata.Decode[openai.EmbeddingNewParams](mergedOpts.Extensions, protocolModalityRequestExtensionKey(e.provider, "embedding"))

	params := &paramsValue
	if err != nil {
		return nil, err
	}

	params.Model = mergedOpts.Model
	params.Input = openai.EmbeddingNewParamsInputUnion{
		OfArrayOfStrings: req.Texts,
	}

	if mergedOpts.Dimensions != nil {
		params.Dimensions = openai.Int(*mergedOpts.Dimensions)
	}

	return params, nil
}

func (e *EmbeddingModel) buildEmbeddingResponse(apiResp *openai.CreateEmbeddingResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Data) == 0 {
		return nil, errors.New("openai: embeddings response has no data")
	}
	if len(apiResp.Data) != expectedResults {
		return nil, fmt.Errorf("openai: embeddings response returned %d results for %d inputs", len(apiResp.Data), expectedResults)
	}

	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &embedding.Usage{
			InputTokens: apiResp.Usage.PromptTokens,
		},
	}

	results := make([]*embedding.Result, len(apiResp.Data))
	seen := make([]bool, len(apiResp.Data))
	for _, item := range apiResp.Data {
		if item.Index < 0 || item.Index >= int64(len(results)) {
			return nil, fmt.Errorf("openai: embeddings response index %d is out of range", item.Index)
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("openai: embeddings response repeats index %d", item.Index)
		}
		resultMeta := &embedding.ResultMetadata{}

		result, err := embedding.NewResult(item.Embedding, resultMeta)
		if err != nil {
			return nil, err
		}

		results[item.Index] = result
		seen[item.Index] = true
	}

	return embedding.NewResponse(results, meta)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := e.buildAPIEmbeddingRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := e.api.Embedding(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildEmbeddingResponse(apiResp, len(req.Texts))
}
