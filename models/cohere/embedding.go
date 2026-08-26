package cohere

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	cohere "github.com/cohere-ai/cohere-go/v2"

	"github.com/Tangerg/lynx/core/embedding"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("cohere: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("cohere: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Cohere's v2 embed endpoint.
//
// Supported models: embed-english-v3.0, embed-multilingual-v3.0,
// embed-english-light-v3.0, embed-multilingual-light-v3.0, embed-v4.0.
// v4 is the only family that supports OutputDimension; older v3 models
// have a fixed 1024-dim output.
type EmbeddingModel struct {
	api            *api
	defaultOptions embedding.Options
}

func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (*cohere.V2EmbedRequest, error) {
	mergedOpts, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	apiRequest, _, err := mergedOpts.Extensions.Decode[cohere.V2EmbedRequest](EmbeddingRequestExtensionKey)
	if err != nil {
		return nil, err
	}
	apiReq := &apiRequest

	apiReq.Model = mergedOpts.Model
	apiReq.Texts = req.Texts

	if apiReq.InputType == "" {
		return nil, fmt.Errorf("cohere: extension %q input_type is required; choose search_document, search_query, classification, or clustering", EmbeddingRequestExtensionKey)
	}

	// Cohere requires at least one embedding type. Core normalizes provider
	// responses to float vectors, so request that wire shape explicitly.
	if len(apiReq.EmbeddingTypes) == 0 {
		apiReq.EmbeddingTypes = []cohere.EmbeddingType{cohere.EmbeddingTypeFloat}
	}

	if mergedOpts.Dimensions != nil {
		value := int(*mergedOpts.Dimensions)
		if int64(value) != *mergedOpts.Dimensions {
			return nil, fmt.Errorf("cohere: embedding: dimensions: %d exceeds int", *mergedOpts.Dimensions)
		}
		apiReq.OutputDimension = &value
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *cohere.EmbedByTypeResponse, expectedResults int) (*embedding.Response, error) {
	if apiResp.Embeddings == nil || len(apiResp.Embeddings.Float) == 0 {
		return nil, errors.New("cohere: embed response has no float embeddings")
	}
	if len(apiResp.Embeddings.Float) != expectedResults {
		return nil, fmt.Errorf("cohere: embed response returned %d outputs for %d inputs", len(apiResp.Embeddings.Float), expectedResults)
	}

	outputs := make([]*embedding.Output, 0, len(apiResp.Embeddings.Float))
	for _, vec := range apiResp.Embeddings.Float {
		outputMetadata := &embedding.OutputMetadata{}

		output, err := embedding.NewOutput(vec, outputMetadata)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}

	meta := &embedding.ResponseMetadata{}
	if apiResp.Meta != nil && apiResp.Meta.BilledUnits != nil {
		usage := new(embedding.Usage)
		if v := apiResp.Meta.BilledUnits.InputTokens; v != nil {
			usage.InputTokens = int64(*v)
		}
		meta.Usage = usage
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
