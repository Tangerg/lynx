package ollama

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Tangerg/lynx/core/embedding"
)

type EmbeddingModelConfig struct {
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.DefaultOptions.Model == "" {
		return errors.New("ollama: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Ollama's /api/embed endpoint. Works with any
// embedding model the daemon has pulled: nomic-embed-text, mxbai-embed-large,
// snowflake-arctic-embed, etc. Use `ollama pull <model>` ahead of time.
type EmbeddingModel struct {
	api            *api
	defaultOptions embedding.Options
}

func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
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

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (*nativeEmbedRequest, error) {
	mergedOpts, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	apiRequest, _, err := mergedOpts.Extensions.Decode[nativeEmbedRequest](EmbeddingRequestExtensionKey)
	if err != nil {
		return nil, err
	}
	apiReq := &apiRequest
	apiReq.Model = mergedOpts.Model
	apiReq.Input = req.Texts

	if mergedOpts.Dimensions != nil {
		dimensions := int(*mergedOpts.Dimensions)
		if int64(dimensions) != *mergedOpts.Dimensions {
			return nil, fmt.Errorf("ollama: embedding: dimensions: %d exceeds int", *mergedOpts.Dimensions)
		}
		apiReq.Dimensions = dimensions
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *nativeEmbedResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Embeddings) == 0 {
		return nil, errors.New("ollama: embed response has no embeddings")
	}
	if len(apiResp.Embeddings) != expectedResults {
		return nil, fmt.Errorf("ollama: embed response returned %d outputs for %d inputs", len(apiResp.Embeddings), expectedResults)
	}

	outputs := make([]*embedding.Output, 0, len(apiResp.Embeddings))
	for _, vec := range apiResp.Embeddings {
		values := make([]float64, len(vec))
		for i, value := range vec {
			values[i] = float64(value)
		}

		outputMetadata := &embedding.OutputMetadata{}

		output, err := embedding.NewOutput(values, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}

	meta := &embedding.ResponseMetadata{
		Model: apiResp.Model,
	}
	if err := meta.Set("ollama/total_duration_ns", apiResp.TotalDuration.Nanoseconds()); err != nil {
		return nil, err
	}
	if err := meta.Set("ollama/load_duration_ns", apiResp.LoadDuration.Nanoseconds()); err != nil {
		return nil, err
	}
	if err := meta.Set("ollama/prompt_eval_count", apiResp.PromptEvalCount); err != nil {
		return nil, err
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

	apiResp, err := e.api.embed(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildResponse(apiResp, len(req.Texts))
}
