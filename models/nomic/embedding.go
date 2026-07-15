package nomic

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
		return errors.New("nomic: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("nomic: DefaultOptions is required")
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
	apiReq.Texts = req.Texts

	if mergedOpts.Dimensions != nil {
		apiReq.Dimensionality = mergedOpts.Dimensions
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *EmbeddingResponse) (*embedding.Response, error) {
	if len(apiResp.Embeddings) == 0 {
		return nil, errors.New("nomic: embedding response has no data")
	}

	textPlain := "text/plain"
	results := make([]*embedding.Result, 0, len(apiResp.Embeddings))
	for i, vec := range apiResp.Embeddings {
		resultMeta := &embedding.ResultMetadata{
			Index:        int64(i),
			ModalityType: embedding.Text,
			MIMEType:     textPlain,
		}

		result, err := embedding.NewResult(vec, resultMeta)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
		Usage: &embedding.Usage{
			PromptTokens: apiResp.Usage.PromptTokens,
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
