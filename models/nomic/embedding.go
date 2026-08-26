package nomic

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Tangerg/lynx/core/embedding"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (e EmbeddingModelConfig) Validate() error {
	if e.APIKey == "" {
		return errors.New("nomic: APIKey is required")
	}
	if e.DefaultOptions.Model == "" {
		return errors.New("nomic: DefaultOptions.Model is required")
	}
	if _, err := e.DefaultOptions.Merged(); err != nil {
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
	mergedOpts, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	apiReqValue, _, err := mergedOpts.Extensions.Decode[embeddingRequest](EmbeddingRequestExtensionKey)

	apiReq := &apiReqValue
	if err != nil {
		return nil, err
	}

	apiReq.Model = mergedOpts.Model
	apiReq.Texts = req.Texts

	if mergedOpts.Dimensions != nil {
		apiReq.Dimensionality = mergedOpts.Dimensions
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *embeddingResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Embeddings) == 0 {
		return nil, errors.New("nomic: embedding response has no data")
	}
	if len(apiResp.Embeddings) != expectedResults {
		return nil, fmt.Errorf("nomic: embedding response returned %d outputs for %d inputs", len(apiResp.Embeddings), expectedResults)
	}

	outputs := make([]*embedding.Output, 0, len(apiResp.Embeddings))
	for _, vec := range apiResp.Embeddings {
		outputMetadata := &embedding.OutputMetadata{}

		output, err := embedding.NewOutput(vec, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
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
