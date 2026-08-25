package voyage

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

	// BaseURL / HTTPClient mirror [APIConfig] for callers that need to
	// proxy through a custom endpoint or share an http.Client.
	BaseURL    string
	HTTPClient *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("voyage: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("voyage: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Voyage AI's /embeddings endpoint. Voyage is
// Anthropic's officially recommended embedding provider, so this gives
// Anthropic-centric stacks a first-class RAG embedder without routing
// through OpenAI/Google.
//
// Current general-purpose models are voyage-4-large, voyage-4, and
// voyage-4-lite. Specialized models such as voyage-code-3 remain supported.
//
// Voyage-specific knobs that don't fit the generic surface — InputType
// ("query" / "document" for asymmetric retrieval), Truncation,
// OutputDtype (int8/uint8/binary quantization) — are reached via the
// extension-threaded SDK params, see [getOptionsParams] and the
// [EmbeddingRequest] struct.
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
	apiReq.Input = req.Texts

	if mergedOpts.Dimensions != nil {
		apiReq.OutputDimension = mergedOpts.Dimensions
	}
	if apiReq.OutputDtype == "" {
		apiReq.OutputDtype = "float"
	}
	if apiReq.OutputDtype != "float" {
		return nil, fmt.Errorf("voyage: extension %q output_dtype %q cannot be represented by Core float embeddings", EmbeddingRequestExtensionKey, apiReq.OutputDtype)
	}
	if apiReq.EncodingFormat != "" {
		return nil, fmt.Errorf("voyage: extension %q encoding_format is unsupported by Core float embeddings", EmbeddingRequestExtensionKey)
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *embeddingResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Data) == 0 {
		return nil, errors.New("voyage: embedding response has no data")
	}
	if len(apiResp.Data) != expectedResults {
		return nil, fmt.Errorf("voyage: embedding response returned %d results for %d inputs", len(apiResp.Data), expectedResults)
	}

	results := make([]*embedding.Result, len(apiResp.Data))
	seen := make([]bool, len(apiResp.Data))
	for _, item := range apiResp.Data {
		if item.Index < 0 || item.Index >= int64(len(results)) {
			return nil, fmt.Errorf("voyage: embedding response index %d is out of range", item.Index)
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("voyage: embedding response repeats index %d", item.Index)
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
			InputTokens: apiResp.Usage.TotalTokens,
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

	apiResp, err := e.api.embedding(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return e.buildResponse(apiResp, len(req.Texts))
}
