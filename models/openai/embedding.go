package openai

import (
	"context"
	"errors"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/options"
	"github.com/Tangerg/lynx/pkg/ptr"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	RequestOptions []option.RequestOption
}

func (c EmbeddingModelConfig) Validate() error {
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
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (e *EmbeddingModel) buildAPIEmbeddingRequest(req *embedding.Request) (*openai.EmbeddingNewParams, error) {
	mergedOpts, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	params, err := options.GetParams[openai.EmbeddingNewParams](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return nil, err
	}

	params.Model = mergedOpts.Model
	params.Input = openai.EmbeddingNewParamsInputUnion{
		OfArrayOfStrings: req.Texts,
	}

	if mergedOpts.Dimensions != nil {
		params.Dimensions = openai.Int(ptr.From(mergedOpts.Dimensions))
	}

	return params, nil
}

func (e *EmbeddingModel) buildEmbeddingResponse(apiResp *openai.CreateEmbeddingResponse) (*embedding.Response, error) {
	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &embedding.Usage{
			InputTokens: apiResp.Usage.PromptTokens,
		},
		Created: time.Now().Unix(),
	}

	results := make([]*embedding.Result, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		resultMeta := &embedding.ResultMetadata{}

		result, err := embedding.NewResult(item.Embedding, resultMeta)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
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

	return e.buildEmbeddingResponse(apiResp)
}
