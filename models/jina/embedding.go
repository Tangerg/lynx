package jina

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/options"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions *embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("jina: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("jina: DefaultOptions is required")
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct {
	api            *API
	defaultOptions *embedding.Options
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
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (*EmbeddingRequest, error) {
	mergedOpts, err := embedding.MergeOptions(e.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[EmbeddingRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}

	apiReq.Model = mergedOpts.Model
	apiReq.Input = req.Texts

	if mergedOpts.Dimensions != nil {
		apiReq.Dimensions = mergedOpts.Dimensions
	}
	if mergedOpts.EncodingFormat.Valid() && apiReq.EmbeddingType == "" {
		apiReq.EmbeddingType = string(mergedOpts.EncodingFormat)
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *EmbeddingResponse) (*embedding.Response, error) {
	if len(apiResp.Data) == 0 {
		return nil, errors.New("jina: embedding response has no data")
	}

	textPlain := "text/plain"
	results := make([]*embedding.Result, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		resultMeta := &embedding.ResultMetadata{
			Index:        item.Index,
			ModalityType: embedding.Text,
			MIMEType:     textPlain,
		}

		result, err := embedding.NewResult(item.Embedding, resultMeta)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &embedding.Usage{
			InputTokens: apiResp.Usage.PromptTokens,
		},
		Created: time.Now().Unix(),
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

	return e.buildResponse(apiResp)
}
