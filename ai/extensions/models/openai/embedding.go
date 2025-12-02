package openai

import (
	"context"
	"errors"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/pkg/mime"
	"github.com/Tangerg/lynx/pkg/ptr"
)

type EmbeddingModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *embedding.Options
	RequestOptions []option.RequestOption
}

func (c *EmbeddingModelConfig) validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("apiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("default options cannot be nil")
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct {
	api            *Api
	defaultOptions *embedding.Options
}

func NewEmbeddingModel(cfg *EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		ApiKey:         cfg.ApiKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (e *EmbeddingModel) buildApiEmbeddingRequest(req *embedding.Request) (*openai.EmbeddingNewParams, error) {
	mergedOpts, err := embedding.MergeOptions(e.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := getOptionsParams[openai.EmbeddingNewParams](mergedOpts)

	params.Model = mergedOpts.Model

	params.Input = openai.EmbeddingNewParamsInputUnion{
		OfArrayOfStrings: req.Texts,
	}

	if mergedOpts.Dimensions != nil {
		params.Dimensions = openai.Int(ptr.Value(mergedOpts.Dimensions))
	}

	if mergedOpts.EncodingFormat.Valid() {
		params.EncodingFormat = openai.EmbeddingNewParamsEncodingFormat(mergedOpts.EncodingFormat)
	}

	return params, nil
}

func (e *EmbeddingModel) buildEmbeddingResponse(apiResp *openai.CreateEmbeddingResponse) (*embedding.Response, error) {
	metadata := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &chat.Usage{
			PromptTokens:  apiResp.Usage.PromptTokens,
			OriginalUsage: apiResp.Usage,
		},
		Created: time.Now().Unix(),
	}

	results := make([]*embedding.Result, 0, len(apiResp.Data))
	for _, embeddingData := range apiResp.Data {
		resultMetadata := &embedding.ResultMetadata{
			Index:        embeddingData.Index,
			ModalityType: embedding.Text,
			MimeType:     mime.MustNew("text", "plain"),
		}

		result, err := embedding.NewResult(embeddingData.Embedding, resultMetadata)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	return embedding.NewResponse(results, metadata)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	apiReq, err := e.buildApiEmbeddingRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := e.api.Embedding(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildEmbeddingResponse(apiResp)
}

func (e *EmbeddingModel) Dimensions(ctx context.Context) int64 {
	return embedding.GetDimensions(ctx, e)
}

func (e *EmbeddingModel) DefaultOptions() *embedding.Options {
	return e.defaultOptions
}

func (e *EmbeddingModel) Info() embedding.ModelInfo {
	return embedding.ModelInfo{
		Provider: Provider,
	}
}
