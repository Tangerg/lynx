package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/Tangerg/lynx/core/embedding"
)

type EmbeddingModelConfig struct {
	DefaultOptions *embedding.Options
	Region         string
	AWSConfig      *aws.Config
}

func (c EmbeddingModelConfig) Validate() error {
	if c.DefaultOptions == nil {
		return errors.New("bedrock: DefaultOptions is required")
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel wraps Bedrock's InvokeModel for embedding model
// families. Each family has its own request/response shape — Titan
// Embed v1/v2 take {"inputText": str}, Cohere Embed v3 takes
// {"texts": [str], "input_type": str}. We dispatch on the model id
// prefix so the same EmbeddingModel can target both.
//
// Bedrock embedding endpoints do NOT batch through a single call (each
// invocation handles one input); Call therefore loops over Request.Texts
// internally.
type EmbeddingModel struct {
	api            *API
	defaultOptions *embedding.Options
}

func NewEmbeddingModel(ctx context.Context, cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(ctx, APIConfig{Region: cfg.Region, AWSConfig: cfg.AWSConfig})
	if err != nil {
		return nil, err
	}
	return &EmbeddingModel{api: api, defaultOptions: cfg.DefaultOptions}, nil
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	results := make([]*embedding.Result, 0, len(req.Texts))
	for index, text := range req.Texts {
		vec, err := e.invoke(ctx, mergedOpts.Model, text, mergedOpts.Dimensions)
		if err != nil {
			return nil, err
		}
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

	return embedding.NewResponse(results, &embedding.ResponseMetadata{
		Model:   mergedOpts.Model,
		Created: time.Now().Unix(),
	})
}

// invoke dispatches the request body shape based on model id prefix.
func (e *EmbeddingModel) invoke(ctx context.Context, modelID, text string, dims *int64) ([]float64, error) {
	body, err := buildEmbeddingBody(modelID, text, dims)
	if err != nil {
		return nil, err
	}

	out, err := e.api.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        body,
	})
	if err != nil {
		return nil, err
	}

	return parseEmbeddingResponse(modelID, out.Body)
}

func buildEmbeddingBody(modelID, text string, dims *int64) ([]byte, error) {
	switch {
	case strings.HasPrefix(modelID, "amazon.titan-embed"):
		req := map[string]any{"inputText": text}
		if dims != nil {
			req["dimensions"] = *dims
		}
		return json.Marshal(req)
	case strings.HasPrefix(modelID, "cohere.embed"):
		req := map[string]any{
			"texts":      []string{text},
			"input_type": "search_document",
		}
		return json.Marshal(req)
	default:
		return nil, errors.New("bedrock: unsupported embedding model: " + modelID)
	}
}

func parseEmbeddingResponse(modelID string, body []byte) ([]float64, error) {
	switch {
	case strings.HasPrefix(modelID, "amazon.titan-embed"):
		var out struct {
			Embedding []float64 `json:"embedding"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, err
		}
		return out.Embedding, nil
	case strings.HasPrefix(modelID, "cohere.embed"):
		var out struct {
			Embeddings [][]float64 `json:"embeddings"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, err
		}
		if len(out.Embeddings) == 0 {
			return nil, errors.New("bedrock: response has no output")
		}
		return out.Embeddings[0], nil
	default:
		return nil, errors.New("bedrock: unsupported embedding model: " + modelID)
	}
}
