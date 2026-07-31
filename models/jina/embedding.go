package jina

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/options"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("jina: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("jina: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct {
	api            *API
	defaultOptions embedding.Options
}

func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (*EmbeddingRequest, error) {
	mergedOpts, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[EmbeddingRequest](mergedOpts.Extensions, EmbeddingRequestExtensionKey)
	if err != nil {
		return nil, err
	}

	apiReq.Model = mergedOpts.Model
	apiReq.Input = req.Texts

	if mergedOpts.Dimensions != nil {
		apiReq.Dimensions = mergedOpts.Dimensions
	}
	if apiReq.EmbeddingType == "" {
		apiReq.EmbeddingType = "float"
	}
	if apiReq.EmbeddingType != "float" {
		return nil, fmt.Errorf("jina: extension %q embedding_type %q cannot be represented by Core float embeddings", EmbeddingRequestExtensionKey, apiReq.EmbeddingType)
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *EmbeddingResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Data) == 0 {
		return nil, errors.New("jina: embedding response has no data")
	}
	if len(apiResp.Data) != expectedResults {
		return nil, fmt.Errorf("jina: embedding response returned %d results for %d inputs", len(apiResp.Data), expectedResults)
	}

	results := make([]*embedding.Result, len(apiResp.Data))
	seen := make([]bool, len(apiResp.Data))
	for _, item := range apiResp.Data {
		if item.Index < 0 || item.Index >= int64(len(results)) {
			return nil, fmt.Errorf("jina: embedding response index %d is out of range", item.Index)
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("jina: embedding response repeats index %d", item.Index)
		}
		resultMeta := &embedding.ResultMetadata{}

		result, err := embedding.NewResult(item.Embedding, resultMeta)
		if err != nil {
			return nil, err
		}
		results[item.Index] = result
		seen[item.Index] = true
	}

	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &embedding.Usage{
			InputTokens: apiResp.Usage.PromptTokens,
		},
	}

	return embedding.NewResponse(results, meta)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := e.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := e.api.Embedding(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildResponse(apiResp, len(req.Texts))
}
