package protocol

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/genai"

	"github.com/Tangerg/scope/core/embedding"
)

type EmbeddingModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions embedding.Options

	// Backend / Project / Location enable Vertex AI access — see
	// the matching fields on [ChatConfig] for semantics.
	Backend  genai.Backend
	Project  string
	Location string

	// BaseURL overrides the genai endpoint. Optional.
	BaseURL string

	HTTPClient *http.Client
}

func (e EmbeddingModelConfig) Validate() error {
	if err := validateProvider(e.Provider); err != nil {
		return fmt.Errorf("google: Provider: %w", err)
	}
	if e.Backend != genai.BackendVertexAI && e.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	if e.DefaultOptions.Model == "" {
		return errors.New("google: DefaultOptions.Model is required")
	}
	if err := e.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Gemini's embed_content endpoint. New integrations use
// gemini-embedding-2, whose output dimensionality is configurable from 128 to
// 3072. Core's text-only request intentionally exposes only that model's text
// input capability; richer multimodal embedding inputs belong in a dedicated
// protocol rather than being hidden inside text.
type EmbeddingModel struct {
	api            *api
	provider       string
	defaultOptions embedding.Options
}

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		Backend:    config.Backend,
		Project:    config.Project,
		Location:   config.Location,
		BaseURL:    config.BaseURL,
		HTTPClient: config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{
		api:            api,
		provider:       config.Provider,
		defaultOptions: config.DefaultOptions.Clone(),
	}, nil
}

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (string, []*genai.Content, *genai.EmbedContentConfig, error) {
	effectiveOptions, err := e.defaultOptions.Resolve(req.Options)
	if err != nil {
		return "", nil, nil, err
	}

	cfgValue, _, err := effectiveOptions.Extensions.Decode[genai.EmbedContentConfig](protocolKey(e.provider, "embedding_request"))

	config := &cfgValue
	if err != nil {
		return "", nil, nil, err
	}

	if effectiveOptions.Dimensions != nil {
		if *effectiveOptions.Dimensions < 128 || *effectiveOptions.Dimensions > 3072 {
			return "", nil, nil, fmt.Errorf("google: embedding: dimensions must be between 128 and 3072: %d", *effectiveOptions.Dimensions)
		}
		config.OutputDimensionality = new(int32(*effectiveOptions.Dimensions))
	}

	contents := make([]*genai.Content, 0, len(req.Texts))
	for _, text := range req.Texts {
		contents = append(contents, genai.NewContentFromText(text, genai.RoleUser))
	}

	return effectiveOptions.Model, contents, config, nil
}

func (e *EmbeddingModel) buildResponse(modelName string, apiResp *genai.EmbedContentResponse, expectedResults int) (*embedding.Response, error) {
	if len(apiResp.Embeddings) == 0 {
		return nil, errors.New("google: embed_content response has no embeddings")
	}
	if len(apiResp.Embeddings) != expectedResults {
		return nil, fmt.Errorf("google: embed_content response returned %d outputs for %d inputs", len(apiResp.Embeddings), expectedResults)
	}

	outputs := make([]*embedding.Output, 0, len(apiResp.Embeddings))
	for _, item := range apiResp.Embeddings {
		values := make([]float64, len(item.Values))
		for i, value := range item.Values {
			values[i] = float64(value)
		}

		outputMetadata := &embedding.OutputMetadata{}
		if item.Statistics != nil {
			if err := outputMetadata.Set(protocolKey(e.provider, "token_count"), item.Statistics.TokenCount); err != nil {
				return nil, err
			}
			if err := outputMetadata.Set(protocolKey(e.provider, "truncated"), item.Statistics.Truncated); err != nil {
				return nil, err
			}
		}

		output, err := embedding.NewOutput(values, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}

	meta := &embedding.ResponseMetadata{
		Model: modelName,
	}
	if err := meta.Set(protocolKey(e.provider, "embedding_response"), apiResp); err != nil {
		return nil, err
	}
	if apiResp.Metadata != nil {
		// Gemini does not report per-modality prompt tokens; surface the
		// billable character count instead so callers can still cost the
		// call.
		if err := meta.Set(protocolKey(e.provider, "billable_character_count"), apiResp.Metadata.BillableCharacterCount); err != nil {
			return nil, err
		}
	}

	return embedding.NewResponse(outputs, meta)
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	modelName, contents, config, err := e.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := e.api.embedding(ctx, modelName, contents, config)
	if err != nil {
		return nil, err
	}

	return e.buildResponse(modelName, apiResp, len(req.Texts))
}
