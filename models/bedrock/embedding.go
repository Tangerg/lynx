package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
)

const (
	// EmbeddingRequestExtensionKey stores [EmbeddingRequestOptions] in Core
	// embedding options.
	EmbeddingRequestExtensionKey = "bedrock/embedding_request"
	// EmbeddingResponseExtensionKey preserves the provider's JSON response body.
	// Titan batching stores one body per input; Cohere stores its single batch body.
	EmbeddingResponseExtensionKey = "bedrock/embedding_response"
)

// EmbeddingRequestOptions carries official family-specific InvokeModel fields
// with no provider-neutral Core equivalent. Unsupported fields are rejected for
// the selected model family rather than silently ignored.
type EmbeddingRequestOptions struct {
	// InputType is required by Cohere Embed and distinguishes retrieval queries,
	// retrieval documents, classification inputs, and clustering inputs.
	InputType string `json:"input_type,omitempty"`
	// Truncate controls Cohere's over-length behavior.
	Truncate string `json:"truncate,omitempty"`
	// Normalize controls Amazon Titan Text Embeddings V2 output normalization.
	Normalize *bool `json:"normalize,omitempty"`
}

type EmbeddingModelConfig struct {
	DefaultOptions embedding.Options
	Region         string
	AWSConfig      *aws.Config
}

func (c EmbeddingModelConfig) Validate() error {
	if c.DefaultOptions.Model == "" {
		return errors.New("bedrock: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

// EmbeddingModel implements the native InvokeModel wire contracts for Amazon
// Titan Text Embeddings V1/V2 and Cohere Embed V3/V4. Titan accepts one input
// per invocation; Cohere accepts up to 96 texts in one batch.
type EmbeddingModel struct {
	api            *API
	defaultOptions embedding.Options
}

func NewEmbeddingModel(ctx context.Context, cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(ctx, APIConfig{Region: cfg.Region, AWSConfig: cfg.AWSConfig})
	if err != nil {
		return nil, err
	}
	return &EmbeddingModel{api: api, defaultOptions: cfg.DefaultOptions.Clone()}, nil
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	merged, err := e.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	nativeValue, _, err := metadata.Decode[EmbeddingRequestOptions](merged.Extensions, EmbeddingRequestExtensionKey)
	if err != nil {
		return nil, err
	}
	native := &nativeValue

	family, err := classifyEmbeddingModel(merged.Model)
	if err != nil {
		return nil, err
	}

	var batch *embeddingBatch
	switch family {
	case embeddingFamilyTitanV1, embeddingFamilyTitanV2:
		batch, err = e.embedTitan(ctx, family, merged.Model, req.Texts, merged.Dimensions, native)
	case embeddingFamilyCohereV3, embeddingFamilyCohereV4:
		batch, err = e.embedCohere(ctx, family, merged.Model, req.Texts, merged.Dimensions, native)
	default:
		return nil, fmt.Errorf("bedrock: unsupported internal embedding family %d", family)
	}
	if err != nil {
		return nil, err
	}

	results := make([]*embedding.Result, len(batch.vectors))
	for index, vector := range batch.vectors {
		result, resultErr := embedding.NewResult(vector, &embedding.ResultMetadata{})
		if resultErr != nil {
			return nil, fmt.Errorf("bedrock: embedding response result %d: %w", index, resultErr)
		}
		results[index] = result
	}

	metadata := &embedding.ResponseMetadata{Model: merged.Model}
	if batch.hasUsage {
		metadata.Usage = &embedding.Usage{InputTokens: batch.inputTokens}
	}
	if len(batch.responseBodies) == 1 {
		if err := metadata.Set(EmbeddingResponseExtensionKey, batch.responseBodies[0]); err != nil {
			return nil, err
		}
	} else if len(batch.responseBodies) > 1 {
		if err := metadata.Set(EmbeddingResponseExtensionKey, batch.responseBodies); err != nil {
			return nil, err
		}
	}
	return embedding.NewResponse(results, metadata)
}

type embeddingFamily uint8

const (
	embeddingFamilyTitanV1 embeddingFamily = iota + 1
	embeddingFamilyTitanV2
	embeddingFamilyCohereV3
	embeddingFamilyCohereV4
)

func classifyEmbeddingModel(modelID string) (embeddingFamily, error) {
	switch {
	case strings.Contains(modelID, "amazon.titan-embed-text-v1"):
		return embeddingFamilyTitanV1, nil
	case strings.Contains(modelID, "amazon.titan-embed-text-v2"):
		return embeddingFamilyTitanV2, nil
	case strings.Contains(modelID, "cohere.embed-v4"):
		return embeddingFamilyCohereV4, nil
	case strings.Contains(modelID, "cohere.embed-english-v3"), strings.Contains(modelID, "cohere.embed-multilingual-v3"):
		return embeddingFamilyCohereV3, nil
	default:
		return 0, fmt.Errorf("bedrock: unsupported embedding model %q", modelID)
	}
}

type embeddingBatch struct {
	vectors        [][]float64
	inputTokens    int64
	hasUsage       bool
	responseBodies []json.RawMessage
}

type titanEmbeddingRequest struct {
	InputText  string `json:"inputText"`
	Dimensions *int64 `json:"dimensions,omitempty"`
	Normalize  *bool  `json:"normalize,omitempty"`
}

type titanEmbeddingResponse struct {
	Embedding           []float64 `json:"embedding"`
	InputTextTokenCount int64     `json:"inputTextTokenCount"`
}

func (e *EmbeddingModel) embedTitan(
	ctx context.Context,
	family embeddingFamily,
	modelID string,
	texts []string,
	dimensions *int64,
	native *EmbeddingRequestOptions,
) (*embeddingBatch, error) {
	if native.InputType != "" {
		return nil, fmt.Errorf("bedrock: embedding extension %q input_type is unsupported by Amazon Titan", EmbeddingRequestExtensionKey)
	}
	if native.Truncate != "" {
		return nil, fmt.Errorf("bedrock: embedding extension %q truncate is unsupported by Amazon Titan", EmbeddingRequestExtensionKey)
	}
	if family == embeddingFamilyTitanV1 {
		if dimensions != nil {
			return nil, errors.New("bedrock: embedding: dimensions are unsupported by Amazon Titan Text Embeddings V1")
		}
		if native.Normalize != nil {
			return nil, fmt.Errorf("bedrock: embedding extension %q normalize is unsupported by Amazon Titan Text Embeddings V1", EmbeddingRequestExtensionKey)
		}
	}
	if family == embeddingFamilyTitanV2 {
		if err := validateDimensions("Amazon Titan Text Embeddings V2", dimensions, 256, 512, 1024); err != nil {
			return nil, err
		}
	}

	batch := &embeddingBatch{
		vectors:        make([][]float64, 0, len(texts)),
		hasUsage:       true,
		responseBodies: make([]json.RawMessage, 0, len(texts)),
	}
	for index, text := range texts {
		request := titanEmbeddingRequest{InputText: text}
		if family == embeddingFamilyTitanV2 {
			request.Dimensions = dimensions
			request.Normalize = native.Normalize
		}
		body, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("bedrock: encode Titan embedding request %d: %w", index, err)
		}
		responseBody, err := e.invokeEmbedding(ctx, modelID, body)
		if err != nil {
			return nil, fmt.Errorf("bedrock: invoke Titan embedding request %d: %w", index, err)
		}
		var response titanEmbeddingResponse
		if err := json.Unmarshal(responseBody, &response); err != nil {
			return nil, fmt.Errorf("bedrock: decode Titan embedding response %d: %w", index, err)
		}
		if len(response.Embedding) == 0 {
			return nil, fmt.Errorf("bedrock: Titan embedding response %d has no float embedding", index)
		}
		if response.InputTextTokenCount < 0 {
			return nil, fmt.Errorf("bedrock: Titan embedding response %d has negative inputTextTokenCount", index)
		}
		batch.vectors = append(batch.vectors, response.Embedding)
		batch.inputTokens += response.InputTextTokenCount
		batch.responseBodies = append(batch.responseBodies, cloneRawJSON(responseBody))
	}
	return batch, nil
}

type cohereEmbeddingRequest struct {
	Texts           []string `json:"texts"`
	InputType       string   `json:"input_type"`
	Truncate        string   `json:"truncate,omitempty"`
	OutputDimension *int64   `json:"output_dimension,omitempty"`
}

type cohereEmbeddingResponse struct {
	Embeddings json.RawMessage `json:"embeddings"`
}

func (e *EmbeddingModel) embedCohere(
	ctx context.Context,
	family embeddingFamily,
	modelID string,
	texts []string,
	dimensions *int64,
	native *EmbeddingRequestOptions,
) (*embeddingBatch, error) {
	if len(texts) > 96 {
		return nil, fmt.Errorf("bedrock: Cohere Embed accepts at most 96 texts per request; got %d", len(texts))
	}
	if err := validateCohereInputType(native.InputType); err != nil {
		return nil, err
	}
	if err := validateCohereTruncate(family, native.Truncate); err != nil {
		return nil, err
	}
	if native.Normalize != nil {
		return nil, fmt.Errorf("bedrock: embedding extension %q normalize is unsupported by Cohere Embed", EmbeddingRequestExtensionKey)
	}
	if family == embeddingFamilyCohereV3 && dimensions != nil {
		return nil, errors.New("bedrock: embedding: dimensions are unsupported by Cohere Embed V3")
	}
	if family == embeddingFamilyCohereV4 {
		if err := validateDimensions("Cohere Embed V4", dimensions, 256, 512, 1024, 1536); err != nil {
			return nil, err
		}
	}

	request := cohereEmbeddingRequest{
		Texts:     texts,
		InputType: native.InputType,
		Truncate:  native.Truncate,
	}
	if family == embeddingFamilyCohereV4 {
		request.OutputDimension = dimensions
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("bedrock: encode Cohere embedding request: %w", err)
	}
	responseBody, err := e.invokeEmbedding(ctx, modelID, body)
	if err != nil {
		return nil, fmt.Errorf("bedrock: invoke Cohere embedding request: %w", err)
	}

	var response cohereEmbeddingResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("bedrock: decode Cohere embedding response: %w", err)
	}
	vectors, err := decodeCohereFloatEmbeddings(response.Embeddings)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("bedrock: Cohere embedding response returned %d results for %d inputs", len(vectors), len(texts))
	}
	return &embeddingBatch{
		vectors:        vectors,
		responseBodies: []json.RawMessage{cloneRawJSON(responseBody)},
	}, nil
}

func validateCohereInputType(inputType string) error {
	switch inputType {
	case "search_document", "search_query", "classification", "clustering":
		return nil
	case "":
		return fmt.Errorf("bedrock: embedding extension %q input_type is required for Cohere Embed", EmbeddingRequestExtensionKey)
	default:
		return fmt.Errorf("bedrock: embedding extension %q has invalid Cohere input_type %q", EmbeddingRequestExtensionKey, inputType)
	}
}

func validateCohereTruncate(family embeddingFamily, truncate string) error {
	if truncate == "" {
		return nil
	}
	if family == embeddingFamilyCohereV3 {
		switch truncate {
		case "NONE", "START", "END":
			return nil
		}
		return fmt.Errorf("bedrock: embedding extension %q has invalid Cohere V3 truncate %q", EmbeddingRequestExtensionKey, truncate)
	}
	switch truncate {
	case "NONE", "LEFT", "RIGHT":
		return nil
	default:
		return fmt.Errorf("bedrock: embedding extension %q has invalid Cohere V4 truncate %q", EmbeddingRequestExtensionKey, truncate)
	}
}

func validateDimensions(model string, dimensions *int64, allowed ...int64) error {
	if dimensions == nil {
		return nil
	}
	for _, value := range allowed {
		if *dimensions == value {
			return nil
		}
	}
	return fmt.Errorf("bedrock: embedding: dimensions %d are unsupported by %s; allowed values are %v", *dimensions, model, allowed)
}

func decodeCohereFloatEmbeddings(raw json.RawMessage) ([][]float64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("bedrock: Cohere embedding response has no embeddings")
	}
	var vectors [][]float64
	if err := json.Unmarshal(raw, &vectors); err == nil {
		if len(vectors) == 0 {
			return nil, errors.New("bedrock: Cohere embedding response has no float embeddings")
		}
		return vectors, nil
	}
	var byType struct {
		Float [][]float64 `json:"float"`
	}
	if err := json.Unmarshal(raw, &byType); err != nil {
		return nil, fmt.Errorf("bedrock: decode Cohere float embeddings: %w", err)
	}
	if len(byType.Float) == 0 {
		return nil, errors.New("bedrock: Cohere embedding response has no float embeddings")
	}
	return byType.Float, nil
}

func (e *EmbeddingModel) invokeEmbedding(ctx context.Context, modelID string, body []byte) ([]byte, error) {
	output, err := e.api.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        body,
	})
	if err != nil {
		return nil, err
	}
	if len(output.Body) == 0 {
		return nil, errors.New("bedrock: InvokeModel returned an empty response body")
	}
	if !json.Valid(output.Body) {
		return nil, errors.New("bedrock: InvokeModel returned invalid JSON")
	}
	return output.Body, nil
}

func cloneRawJSON(value []byte) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
