package voyage

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

	// BaseURL / HTTPClient mirror [APIConfig] for callers that need to
	// proxy through a custom endpoint or share an http.Client.
	BaseURL    string
	HTTPClient *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("voyage: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("voyage: DefaultOptions is required")
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Voyage AI's /embeddings endpoint. Voyage is
// Anthropic's officially recommended embedding provider, so this gives
// Anthropic-centric stacks a first-class RAG embedder without routing
// through OpenAI/Google.
//
// Supported models include voyage-3-large, voyage-3, voyage-3-lite,
// voyage-code-3, voyage-finance-2, voyage-law-2, voyage-multilingual-2.
//
// Voyage-specific knobs that don't fit the generic surface — InputType
// ("query" / "document" for asymmetric retrieval), Truncation,
// OutputDtype (int8/uint8/binary quantization) — are reached via the
// Extra-threaded SDK params, see [getOptionsParams] and the
// [EmbeddingRequest] struct.
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
		apiReq.OutputDimension = mergedOpts.Dimensions
	}
	if mergedOpts.EncodingFormat.Valid() {
		apiReq.EncodingFormat = string(mergedOpts.EncodingFormat)
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *EmbeddingResponse) (*embedding.Response, error) {
	if len(apiResp.Data) == 0 {
		return nil, errors.New("voyage: embedding response has no data")
	}

	results := make([]*embedding.Result, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		resultMeta := &embedding.ResultMetadata{
			Index:        item.Index,
			ModalityType: embedding.Text,
			MIMEType:     "text/plain",
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
			PromptTokens: apiResp.Usage.TotalTokens,
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
