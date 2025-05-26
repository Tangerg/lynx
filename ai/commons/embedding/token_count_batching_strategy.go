package embedding

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/ai/commons/document"
	"github.com/Tangerg/lynx/ai/commons/tokenizer"
	"math"
	"slices"
)

const (
	// MaxInputTokenCount uses OpenAI upper limit of input token count as the default.
	MaxInputTokenCount = 8191

	// DefaultTokenCountReservePercentage is the default percentage of tokens to reserve
	// when calculating the actual max input token count.
	DefaultTokenCountReservePercentage = 0.1
)

var _ BatchingStrategy = (*TokenCountBatchingStrategy)(nil)

// TokenCountBatchingStrategy is a token count based strategy implementation for BatchingStrategy.
// Using OpenAI max input token as the default: https://platform.openai.com/docs/guides/embeddings/embedding-models.
//
// This strategy incorporates a reserve percentage to provide a buffer for potential
// overhead or unexpected increases in token count during processing. The actual max input
// token count used is calculated as: actualMaxInputTokenCount = originalMaxInputTokenCount * (1 - RESERVE_PERCENTAGE)
//
// For example, with the default reserve percentage of 10% (DefaultTokenCountReservePercentage)
// and the default max input token count of 8191 (MaxInputTokenCount), the actual max input
// token count used will be 7371.
//
// The strategy batches documents based on their token counts, ensuring that each batch
// does not exceed the calculated max input token count.
type TokenCountBatchingStrategy struct {
	tokenCountEstimator tokenizer.TokenCountEstimator
	maxInputTokenCount  int
	contentFormatter    document.ContentFormatter
	metadataMode        document.MetadataMode
}

// NewTokenCountBatchingStrategy creates a TokenCountBatchingStrategy with default values
// and the specified token count estimator. This is a convenience method equivalent to
// using the builder with all default values.
//
// Parameters:
//
//	tokenCountEstimator - the TokenCountEstimator to be used for estimating token counts
//
// Returns:
//
//	A configured TokenCountBatchingStrategy instance, or an error if the estimator is nil.
func NewTokenCountBatchingStrategy(tokenCountEstimator tokenizer.TokenCountEstimator) (*TokenCountBatchingStrategy, error) {
	return NewTokenCountBatchingStrategyBuilder().
		WithTokenCountEstimator(tokenCountEstimator).
		Build()
}

// Batch optimizes embedding tokens by splitting the incoming collection of Documents into sub-batches
// based on their token counts. Each batch does not exceed the calculated max input token count.
// The order of documents is preserved during batching as they are mapped to their corresponding
// embeddings by their order.
//
// Parameters:
//
//	ctx - the context for cancellation and timeout control
//	docs - documents to batch
//
// Returns:
//
//	A list of sub-batches that contain Documents, or an error if batching fails.
func (t *TokenCountBatchingStrategy) Batch(ctx context.Context, docs []*document.Document) ([][]*document.Document, error) {
	type docWithTokens struct {
		doc    *document.Document
		tokens int
	}
	var dwts []*docWithTokens
	for _, doc := range docs {
		content := doc.FormattedContentByMetadataModeWithContentFormatter(t.metadataMode, t.contentFormatter)
		tokenCount, err := t.tokenCountEstimator.EstimateText(ctx, content)
		if err != nil {
			return nil, err
		}
		if tokenCount > t.maxInputTokenCount {
			return nil, errors.New("tokens in a single document exceeds the maximum number of allowed input tokens")
		}
		dwts = append(dwts, &docWithTokens{doc: doc, tokens: tokenCount})
	}

	var (
		currentSize  = 0
		batches      [][]*document.Document
		currentBatch []*document.Document
	)
	for _, dwt := range dwts {
		if currentSize+dwt.tokens > t.maxInputTokenCount {
			batches = append(batches, currentBatch)
			currentBatch = make([]*document.Document, 0)
			currentSize = 0
		}
		currentBatch = append(currentBatch, dwt.doc)
		currentSize += dwt.tokens
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches, nil
}

// TokenCountBatchingStrategyBuilder is a builder pattern implementation for creating TokenCountBatchingStrategy instances.
// It provides a fluent API for configuring various parameters of the batching strategy.
type TokenCountBatchingStrategyBuilder struct {
	tokenCountEstimator tokenizer.TokenCountEstimator
	maxInputTokenCount  int
	reservePercentage   float64
	contentFormatter    document.ContentFormatter
	metadataMode        document.MetadataMode
}

// NewTokenCountBatchingStrategyBuilder creates a new TokenCountBatchingStrategyBuilder with default values.
// The builder is initialized with:
//   - maxInputTokenCount: 8191 (OpenAI's upper limit for input tokens)
//   - reservePercentage: 0.1 (10% buffer for potential token count increases)
//   - contentFormatter: default content formatter
//   - metadataMode: MetadataModeNone (no metadata processing)
//
// Only the tokenCountEstimator parameter is left uninitialized and must be provided
// before calling Build(), as it is a required parameter.
//
// Returns:
//
//	A new TokenCountBatchingStrategyBuilder instance with default configuration.
func NewTokenCountBatchingStrategyBuilder() *TokenCountBatchingStrategyBuilder {
	return &TokenCountBatchingStrategyBuilder{
		maxInputTokenCount: MaxInputTokenCount,
		reservePercentage:  DefaultTokenCountReservePercentage,
		contentFormatter:   document.NewDefaultContentFormatterBuilder().Build(),
		metadataMode:       document.MetadataModeNone,
	}
}

// WithTokenCountEstimator sets the token count estimator for the batching strategy.
// If the provided estimator is nil, this method does nothing and returns the builder for chaining.
//
// Parameters:
//
//	tokenCountEstimator - the TokenCountEstimator to be used for estimating token counts
//
// Returns:
//
//	The builder instance for method chaining.
func (b *TokenCountBatchingStrategyBuilder) WithTokenCountEstimator(tokenCountEstimator tokenizer.TokenCountEstimator) *TokenCountBatchingStrategyBuilder {
	if tokenCountEstimator != nil {
		b.tokenCountEstimator = tokenCountEstimator
	}
	return b
}

// WithMaxInputTokenCount sets the maximum input token count for the batching strategy.
// If the provided count is not positive, this method does nothing and returns the builder for chaining.
//
// Parameters:
//
//	maxInputTokenCount - the upper limit for input tokens (must be greater than 0)
//
// Returns:
//
//	The builder instance for method chaining.
func (b *TokenCountBatchingStrategyBuilder) WithMaxInputTokenCount(maxInputTokenCount int) *TokenCountBatchingStrategyBuilder {
	if maxInputTokenCount > 0 {
		b.maxInputTokenCount = maxInputTokenCount
	}
	return b
}

// WithReservePercentage sets the reserve percentage for the batching strategy.
// The reserve percentage is used to create a buffer by reducing the effective max token count.
// If the provided percentage is not in the valid range [0, 1), this method does nothing.
//
// Parameters:
//
//	reservePercentage - the percentage of tokens to reserve (must be in range [0, 1))
//
// Returns:
//
//	The builder instance for method chaining.
func (b *TokenCountBatchingStrategyBuilder) WithReservePercentage(reservePercentage float64) *TokenCountBatchingStrategyBuilder {
	if reservePercentage >= 0 && reservePercentage < 1 {
		b.reservePercentage = reservePercentage
	}
	return b
}

// WithContentFormatter sets the content formatter for the batching strategy.
// If the provided formatter is nil, this method does nothing and returns the builder for chaining.
//
// Parameters:
//
//	contentFormatter - the ContentFormatter to be used for formatting document content
//
// Returns:
//
//	The builder instance for method chaining.
func (b *TokenCountBatchingStrategyBuilder) WithContentFormatter(contentFormatter document.ContentFormatter) *TokenCountBatchingStrategyBuilder {
	if contentFormatter != nil {
		b.contentFormatter = contentFormatter
	}
	return b
}

// WithMetadataMode sets the metadata mode for the batching strategy.
// The metadata mode determines how document metadata is handled during content formatting.
// Only valid MetadataMode values are accepted; invalid values are ignored.
//
// Valid values:
//   - MetadataModeAll: include all metadata
//   - MetadataModeEmbed: include only embedding-relevant metadata
//   - MetadataModeInference: include only inference-relevant metadata
//   - MetadataModeNone: exclude all metadata
//
// Parameters:
//
//	metadataMode - the MetadataMode to be used for handling metadata
//
// Returns:
//
//	The builder instance for method chaining.
func (b *TokenCountBatchingStrategyBuilder) WithMetadataMode(metadataMode document.MetadataMode) *TokenCountBatchingStrategyBuilder {
	if slices.Contains([]document.MetadataMode{
		document.MetadataModeAll,
		document.MetadataModeEmbed,
		document.MetadataModeInference,
		document.MetadataModeNone,
	}, metadataMode) {
		b.metadataMode = metadataMode
	}
	return b
}

// Build creates a new TokenCountBatchingStrategy instance with the configured parameters.
// It validates required parameters and sets default values for optional ones.
// The actual max input token count is calculated by applying the reserve percentage
// using rounding to match the Java implementation behavior.
//
// Returns:
//
//	A configured TokenCountBatchingStrategy instance, or an error if required parameters are missing.
//
// Errors:
//
//	Returns an error if the token count estimator is not provided.
func (b *TokenCountBatchingStrategyBuilder) Build() (*TokenCountBatchingStrategy, error) {
	if b.tokenCountEstimator == nil {
		return nil, errors.New("token count estimator must not be null")
	}
	if b.maxInputTokenCount <= 0 {
		return nil, errors.New("max input token count must be greater than 0")
	}
	if b.reservePercentage < 0 || b.reservePercentage >= 1 {
		return nil, errors.New("reserve percentage must be in range [0, 1)")
	}
	if b.contentFormatter == nil {
		return nil, errors.New("content formatter must not be null")
	}

	maxInputTokenCount := int(math.Round(float64(b.maxInputTokenCount) * (1 - b.reservePercentage)))
	return &TokenCountBatchingStrategy{
		tokenCountEstimator: b.tokenCountEstimator,
		maxInputTokenCount:  maxInputTokenCount,
		contentFormatter:    b.contentFormatter,
		metadataMode:        b.metadataMode,
	}, nil
}
