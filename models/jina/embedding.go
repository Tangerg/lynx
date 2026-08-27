package jina

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Tangerg/scope/core/embedding"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (e EmbeddingModelConfig) Validate() error {
	if e.APIKey == "" {
		return errors.New("jina: APIKey is required")
	}
	if e.DefaultOptions.Model == "" {
		return errors.New("jina: DefaultOptions.Model is required")
	}
	if err := e.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct {
	api            *api
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
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (*embeddingRequest, error) {
	effectiveOptions, err := e.defaultOptions.Resolve(req.Options)
	if err != nil {
		return nil, err
	}

	apiReqValue, _, err := effectiveOptions.Extensions.Decode[embeddingRequest](EmbeddingRequestExtensionKey)

	apiReq := &apiReqValue
	if err != nil {
		return nil, err
	}

	apiReq.Model = effectiveOptions.Model
	apiReq.Input = req.Texts

	if effectiveOptions.Dimensions != nil {
		apiReq.Dimensions = effectiveOptions.Dimensions
	}
	if apiReq.EmbeddingType == "" {
		apiReq.EmbeddingType = "float"
	}
	if apiReq.EmbeddingType != "float" {
		return nil, fmt.Errorf("jina: extension %q embedding_type %q cannot be represented by Core float embeddings", EmbeddingRequestExtensionKey, apiReq.EmbeddingType)
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *embeddingResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Data) == 0 {
		return nil, errors.New("jina: embedding response has no data")
	}
	if len(apiResp.Data) != expectedResults {
		return nil, fmt.Errorf("jina: embedding response returned %d outputs for %d inputs", len(apiResp.Data), expectedResults)
	}

	outputs := make([]*embedding.Output, len(apiResp.Data))
	seen := make([]bool, len(apiResp.Data))
	for _, item := range apiResp.Data {
		if item.Index < 0 || item.Index >= int64(len(outputs)) {
			return nil, fmt.Errorf("jina: embedding response index %d is out of range", item.Index)
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("jina: embedding response repeats index %d", item.Index)
		}
		outputMetadata := &embedding.OutputMetadata{}

		output, err := embedding.NewOutput(item.Embedding, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs[item.Index] = output
		seen[item.Index] = true
	}

	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &embedding.Usage{
			InputTokens: apiResp.Usage.PromptTokens,
		},
	}

	return embedding.NewResponse(outputs, meta)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := e.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := e.api.embedding(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildResponse(apiResp, len(req.Texts))
}
