package document

import (
	"context"
	"errors"
	"math"
	"slices"

	"github.com/Tangerg/lynx/ai/tokenizer"
)

// BatchingStrategy defines an interface for batching documents to optimize embedding operations.
// Implementations should preserve document order to maintain correct embedding mappings.
type BatchingStrategy interface {
	// Batch splits documents into optimized sub-batches for embedding processing.
	// Document order must be preserved for correct embedding-to-document mapping.
	Batch(ctx context.Context, docs []*Document) ([][]*Document, error)
}

const (
	MaxInputTokenCount                 = 8191
	DefaultTokenCountReservePercentage = 0.1
)

var _ BatchingStrategy = (*TokenCountBatchingStrategy)(nil)

// TokenCountBatchingStrategy batches documents based on token count limits.
// Uses a reserve percentage to provide buffer for token count variations.
type TokenCountBatchingStrategy struct {
	tokenCountEstimator tokenizer.TokenCountEstimator
	maxInputTokenCount  int
	formatter           Formatter
	metadataMode        MetadataMode
}

func NewTokenCountBatchingStrategy(tokenCountEstimator tokenizer.TokenCountEstimator) (*TokenCountBatchingStrategy, error) {
	return NewTokenCountBatchingStrategyBuilder().
		WithTokenCountEstimator(tokenCountEstimator).
		Build()
}

// Batch splits documents into token-limited batches while preserving order.
func (s *TokenCountBatchingStrategy) Batch(ctx context.Context, docs []*Document) ([][]*Document, error) {
	type docWithTokens struct {
		doc    *Document
		tokens int
	}

	var documentsWithTokens []*docWithTokens
	for _, doc := range docs {
		content := doc.FormatByMetadataModeWithFormatter(s.metadataMode, s.formatter)

		tokenCount, err := s.tokenCountEstimator.EstimateText(ctx, content)
		if err != nil {
			return nil, err
		}

		if tokenCount > s.maxInputTokenCount {
			return nil, errors.New("document token count exceeds maximum allowed input tokens")
		}

		documentsWithTokens = append(documentsWithTokens, &docWithTokens{
			doc:    doc,
			tokens: tokenCount,
		})
	}

	var (
		currentTokens = 0
		batches       [][]*Document
		currentBatch  []*Document
	)

	for _, docWithToken := range documentsWithTokens {
		if currentTokens+docWithToken.tokens > s.maxInputTokenCount {
			batches = append(batches, currentBatch)
			currentBatch = make([]*Document, 0)
			currentTokens = 0
		}

		currentBatch = append(currentBatch, docWithToken.doc)
		currentTokens += docWithToken.tokens
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches, nil
}

// TokenCountBatchingStrategyBuilder provides fluent API for creating TokenCountBatchingStrategy instances.
type TokenCountBatchingStrategyBuilder struct {
	tokenCountEstimator tokenizer.TokenCountEstimator
	maxInputTokenCount  int
	reservePercentage   float64
	formatter           Formatter
	metadataMode        MetadataMode
}

func NewTokenCountBatchingStrategyBuilder() *TokenCountBatchingStrategyBuilder {
	return &TokenCountBatchingStrategyBuilder{
		maxInputTokenCount: MaxInputTokenCount,
		reservePercentage:  DefaultTokenCountReservePercentage,
		formatter:          NewDefaultFormatterBuilder().Build(),
		metadataMode:       MetadataModeNone,
	}
}

func (b *TokenCountBatchingStrategyBuilder) WithTokenCountEstimator(tokenCountEstimator tokenizer.TokenCountEstimator) *TokenCountBatchingStrategyBuilder {
	if tokenCountEstimator != nil {
		b.tokenCountEstimator = tokenCountEstimator
	}
	return b
}

func (b *TokenCountBatchingStrategyBuilder) WithMaxInputTokenCount(maxInputTokenCount int) *TokenCountBatchingStrategyBuilder {
	if maxInputTokenCount > 0 {
		b.maxInputTokenCount = maxInputTokenCount
	}
	return b
}

func (b *TokenCountBatchingStrategyBuilder) WithReservePercentage(reservePercentage float64) *TokenCountBatchingStrategyBuilder {
	if reservePercentage >= 0 && reservePercentage < 1 {
		b.reservePercentage = reservePercentage
	}
	return b
}

func (b *TokenCountBatchingStrategyBuilder) WithFormatter(formatter Formatter) *TokenCountBatchingStrategyBuilder {
	if formatter != nil {
		b.formatter = formatter
	}
	return b
}

// WithMetadataMode sets how document metadata is handled during formatting.
func (b *TokenCountBatchingStrategyBuilder) WithMetadataMode(metadataMode MetadataMode) *TokenCountBatchingStrategyBuilder {
	validModes := []MetadataMode{
		MetadataModeAll,
		MetadataModeEmbed,
		MetadataModeInference,
		MetadataModeNone,
	}

	if slices.Contains(validModes, metadataMode) {
		b.metadataMode = metadataMode
	}
	return b
}

// Build creates a TokenCountBatchingStrategy with configured parameters and calculated token limits.
func (b *TokenCountBatchingStrategyBuilder) Build() (*TokenCountBatchingStrategy, error) {
	if b.tokenCountEstimator == nil {
		return nil, errors.New("token count estimator is required")
	}
	if b.maxInputTokenCount <= 0 {
		return nil, errors.New("max input token count must be greater than 0")
	}
	if b.reservePercentage < 0 || b.reservePercentage >= 1 {
		return nil, errors.New("reserve percentage must be in range [0, 1)")
	}
	if b.formatter == nil {
		return nil, errors.New("formatter is required")
	}

	actualMaxTokens := int(math.Round(float64(b.maxInputTokenCount) * (1 - b.reservePercentage)))

	return &TokenCountBatchingStrategy{
		tokenCountEstimator: b.tokenCountEstimator,
		maxInputTokenCount:  actualMaxTokens,
		formatter:           b.formatter,
		metadataMode:        b.metadataMode,
	}, nil
}
