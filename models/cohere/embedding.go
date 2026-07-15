package cohere

import (
	"context"
	"errors"
	"time"

	cohere "github.com/cohere-ai/cohere-go/v2"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/options"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions *embedding.Options
	BaseURL        string
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("cohere: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("cohere: DefaultOptions is required")
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
	api            *API
	defaultOptions *embedding.Options
}

func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL})
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (e *EmbeddingModel) buildAPIRequest(req *embedding.Request) (*cohere.V2EmbedRequest, error) {
	mergedOpts, err := embedding.MergeOptions(e.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[cohere.V2EmbedRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}

	apiReq.Model = mergedOpts.Model
	apiReq.Texts = req.Texts

	// Cohere requires input_type. Default to search_document for the
	// most common (RAG indexing) case when the caller didn't set one
	// via Extra.
	if apiReq.InputType == "" {
		apiReq.InputType = cohere.EmbedInputTypeSearchDocument
	}

	// Cohere requires at least one embedding_type. Map our common
	// EncodingFormat onto the matching Cohere EmbeddingType; default to
	// "float" so downstream code always gets a populated Float field.
	if len(apiReq.EmbeddingTypes) == 0 {
		var t cohere.EmbeddingType
		switch mergedOpts.EncodingFormat {
		case embedding.EncodingFormatBase64:
			t = cohere.EmbeddingTypeBase64
		default:
			t = cohere.EmbeddingTypeFloat
		}
		apiReq.EmbeddingTypes = []cohere.EmbeddingType{t}
	}

	if mergedOpts.Dimensions != nil {
		v := int(*mergedOpts.Dimensions)
		apiReq.OutputDimension = &v
	}

	return apiReq, nil
}

func (e *EmbeddingModel) buildResponse(apiResp *cohere.EmbedByTypeResponse) (*embedding.Response, error) {
	if apiResp.Embeddings == nil || len(apiResp.Embeddings.Float) == 0 {
		return nil, errors.New("cohere: embed response has no float embeddings")
	}

	results := make([]*embedding.Result, 0, len(apiResp.Embeddings.Float))
	for index, vec := range apiResp.Embeddings.Float {
		resultMeta := &embedding.ResultMetadata{
			Index:        int64(index),
			ModalityType: embedding.Text,
			MIMEType:     "text/plain",
		}

		result, err := embedding.NewResult(vec, resultMeta)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := &embedding.ResponseMetadata{
		Created: time.Now().Unix(),
	}
	if apiResp.Meta != nil && apiResp.Meta.BilledUnits != nil {
		usage := new(model.Usage)
		if v := apiResp.Meta.BilledUnits.InputTokens; v != nil {
			usage.PromptTokens = int64(*v)
		}
		meta.Usage = usage
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
