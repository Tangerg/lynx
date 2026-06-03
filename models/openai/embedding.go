package openai

import (
	"context"
	"errors"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/internal/options"
	"github.com/Tangerg/lynx/pkg/mime"
	"github.com/Tangerg/lynx/pkg/ptr"
)

type EmbeddingModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *embedding.Options
	RequestOptions []option.RequestOption

	// Metadata overrides the [embedding.ModelMetadata] returned by [EmbeddingModel.Metadata].
	// Facades pass their own Provider here so observability tags the
	// call by the real upstream brand. Zero Provider falls back to
	// the package default [Provider].
	Metadata *embedding.ModelMetadata
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: DefaultOptions is required")
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct {
	api            *API
	defaultOptions *embedding.Options
	metadata       embedding.ModelMetadata
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

	info := embedding.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}
	return &EmbeddingModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		metadata:       info,
	}, nil
}

func (e *EmbeddingModel) buildAPIEmbeddingRequest(req *embedding.Request) (*openai.EmbeddingNewParams, error) {
	mergedOpts, err := embedding.MergeOptions(e.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[openai.EmbeddingNewParams](mergedOpts, OptionsKey)

	params.Model = mergedOpts.Model
	params.Input = openai.EmbeddingNewParamsInputUnion{
		OfArrayOfStrings: req.Texts,
	}

	if mergedOpts.Dimensions != nil {
		params.Dimensions = openai.Int(ptr.From(mergedOpts.Dimensions))
	}
	if mergedOpts.EncodingFormat.Valid() {
		params.EncodingFormat = openai.EmbeddingNewParamsEncodingFormat(mergedOpts.EncodingFormat)
	}

	return params, nil
}

func (e *EmbeddingModel) buildEmbeddingResponse(apiResp *openai.CreateEmbeddingResponse) (*embedding.Response, error) {
	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &chat.Usage{
			PromptTokens:  apiResp.Usage.PromptTokens,
			OriginalUsage: apiResp.Usage,
		},
		Created: time.Now().Unix(),
	}

	results := make([]*embedding.Result, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		resultMeta := &embedding.ResultMetadata{
			Index:        item.Index,
			ModalityType: embedding.Text,
			MimeType:     mime.MustNew("text", "plain"),
		}

		result, err := embedding.NewResult(item.Embedding, resultMeta)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	return embedding.NewResponse(results, meta)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
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

func (e *EmbeddingModel) Dimensions(ctx context.Context) int64 {
	return embedding.GetDimensions(ctx, e)
}

func (e *EmbeddingModel) DefaultOptions() embedding.Options {
	return *e.defaultOptions
}

func (e *EmbeddingModel) Metadata() embedding.ModelMetadata {
	return e.metadata
}
