package ollama

import (
	"context"
	"errors"
	"net/http"
	"time"

	ollamaapi "github.com/ollama/ollama/api"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/options"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

type EmbeddingModelConfig struct {
	DefaultOptions *embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.DefaultOptions == nil {
		return errors.New("ollama: DefaultOptions is required")
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Ollama's /api/embed endpoint. Works with any
// embedding model the daemon has pulled: nomic-embed-text, mxbai-embed-large,
// snowflake-arctic-embed, etc. Use `ollama pull <model>` ahead of time.
type EmbeddingModel struct {
	api            *API
	defaultOptions *embedding.Options
}

func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
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

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (*ollamaapi.EmbedRequest, error) {
	mergedOpts, err := embedding.MergeOptions(e.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[ollamaapi.EmbedRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}
	apiReq.Model = mergedOpts.Model
	apiReq.Input = req.Texts

	if mergedOpts.Dimensions != nil {
		apiReq.Dimensions = int(*mergedOpts.Dimensions)
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *ollamaapi.EmbedResponse) (*embedding.Response, error) {
	if len(apiResp.Embeddings) == 0 {
		return nil, errors.New("ollama: embed response has no embeddings")
	}

	results := make([]*embedding.Result, 0, len(apiResp.Embeddings))
	for index, vec := range apiResp.Embeddings {
		values := pkgSlices.Map(vec, func(v float32) float64 { return float64(v) })

		resultMeta := &embedding.ResultMetadata{
			Index:        int64(index),
			ModalityType: embedding.Text,
			MIMEType:     "text/plain",
		}

		result, err := embedding.NewResult(values, resultMeta)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := &embedding.ResponseMetadata{
		Model:   apiResp.Model,
		Created: time.Now().Unix(),
	}
	if err := meta.Set("total_duration_ms", apiResp.TotalDuration.Milliseconds()); err != nil {
		return nil, err
	}
	if err := meta.Set("load_duration_ms", apiResp.LoadDuration.Milliseconds()); err != nil {
		return nil, err
	}
	if err := meta.Set("prompt_eval_count", apiResp.PromptEvalCount); err != nil {
		return nil, err
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

	apiResp, err := e.api.Embed(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildResponse(apiResp)
}
