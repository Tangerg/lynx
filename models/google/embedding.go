package google

import (
	"context"
	"errors"
	"time"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/internal/options"
	"github.com/Tangerg/lynx/pkg/mime"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

type EmbeddingModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *embedding.Options

	// Backend / Project / Location enable Vertex AI access — see
	// the matching fields on [ChatModelConfig] for semantics.
	Backend  genai.Backend
	Project  string
	Location string

	// BaseURL overrides the genai endpoint. Optional.
	BaseURL string

	// Metadata overrides the [embedding.ModelMetadata] returned by [EmbeddingModel.Metadata].
	// Zero Provider falls back to [Provider].
	Metadata *embedding.ModelMetadata
}

func (c EmbeddingModelConfig) Validate() error {
	if c.Backend != genai.BackendVertexAI && c.APIKey == nil {
		return errors.New("google: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("google: DefaultOptions is required")
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Gemini's embed_content endpoint. Supported models:
//   - "gemini-embedding-001" (3072 dims; supports OutputDimensionality
//     truncation to 128/256/...);
//   - "text-embedding-004" (768 dims; legacy, no OutputDimensionality).
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
		APIKey:   cfg.APIKey,
		Backend:  cfg.Backend,
		Project:  cfg.Project,
		Location: cfg.Location,
		BaseURL:  cfg.BaseURL,
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

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (string, []*genai.Content, *genai.EmbedContentConfig, error) {
	mergedOpts, err := embedding.MergeOptions(e.defaultOptions, req.Options)
	if err != nil {
		return "", nil, nil, err
	}

	cfg := options.GetParams[genai.EmbedContentConfig](mergedOpts, OptionsKey)

	if mergedOpts.Dimensions != nil {
		cfg.OutputDimensionality = new(int32(*mergedOpts.Dimensions))
	}

	contents := make([]*genai.Content, 0, len(req.Texts))
	for _, text := range req.Texts {
		contents = append(contents, genai.NewContentFromText(text, genai.RoleUser))
	}

	return mergedOpts.Model, contents, cfg, nil
}

func (e *EmbeddingModel) buildResponse(modelName string, apiResp *genai.EmbedContentResponse) (*embedding.Response, error) {
	if len(apiResp.Embeddings) == 0 {
		return nil, errors.New("google: embed_content response has no embeddings")
	}

	results := make([]*embedding.Result, 0, len(apiResp.Embeddings))
	for index, item := range apiResp.Embeddings {
		values := pkgSlices.Map(item.Values, func(v float32) float64 { return float64(v) })

		resultMeta := &embedding.ResultMetadata{
			Index:        int64(index),
			ModalityType: embedding.Text,
			MimeType:     mime.MustNew("text", "plain"),
		}
		if item.Statistics != nil {
			resultMeta.Set("token_count", item.Statistics.TokenCount)
			resultMeta.Set("truncated", item.Statistics.Truncated)
		}

		result, err := embedding.NewResult(values, resultMeta)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := &embedding.ResponseMetadata{
		Model:   modelName,
		Created: time.Now().Unix(),
	}
	if apiResp.Metadata != nil {
		// Gemini does not report per-modality prompt tokens; surface the
		// billable character count instead so callers can still cost the
		// call.
		meta.Set("billable_character_count", apiResp.Metadata.BillableCharacterCount)
	}

	return embedding.NewResponse(results, meta)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	modelName, contents, cfg, err := e.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := e.api.Embedding(ctx, modelName, contents, cfg)
	if err != nil {
		return nil, err
	}

	return e.buildResponse(modelName, apiResp)
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
