package document

import (
	"context"
	"errors"
	"math"
	"slices"

	"github.com/Tangerg/lynx/ai/tokenizer"
)

// TokenCountBatcherConfig holds the configuration for TokenCountBatcher.
type TokenCountBatcherConfig struct {
	// TokenCountEstimator estimates the number of tokens in the formatted document content.
	// Required. Used to calculate batch sizes based on token limits.
	TokenCountEstimator tokenizer.Estimator

	// MaxInputTokenCount sets the maximum number of tokens allowed per batch.
	// Optional. Defaults to 8191 if not provided.
	// This value will be reduced by ReservePercentage to get the actual working limit.
	MaxInputTokenCount int

	// ReservePercentage specifies the percentage of tokens to reserve as buffer.
	// Optional. Defaults to 0.1 (10%) if not provided.
	// Must be in range [0, 1). The actual max tokens = MaxInputTokenCount * (1 - ReservePercentage).
	// This buffer helps prevent edge cases where token estimation might be slightly off.
	ReservePercentage float64

	// Formatter formats documents before token counting.
	// Required. Used to convert documents to their string representation
	// based on the specified MetadataMode.
	Formatter Formatter

	// MetadataMode determines which metadata fields are included in formatted content.
	// Required. Must be one of: MetadataModeAll, MetadataModeEmbed,
	// MetadataModeInference, or MetadataModeNone.
	// This affects the token count estimation and final batch sizes.
	MetadataMode MetadataMode
}

func (c *TokenCountBatcherConfig) validate() error {
	const (
		maxInputTokenCount = 8191
		reservePercentage  = 0.1
	)

	if c == nil {
		return errors.New("config is required")
	}
	if c.MaxInputTokenCount == 0 {
		c.MaxInputTokenCount = maxInputTokenCount
	}
	if c.ReservePercentage == 0 {
		c.ReservePercentage = reservePercentage
	}

	if c.TokenCountEstimator == nil {
		return errors.New("token count estimator is required")
	}
	if c.MaxInputTokenCount <= 0 {
		return errors.New("max input token count must be greater than 0")
	}
	if c.ReservePercentage < 0 || c.ReservePercentage >= 1 {
		return errors.New("reserve percentage must be in range [0, 1)")
	}
	if c.Formatter == nil {
		return errors.New("formatter is required")
	}

	validModes := []MetadataMode{
		MetadataModeAll,
		MetadataModeEmbed,
		MetadataModeInference,
		MetadataModeNone,
	}
	if !slices.Contains(validModes, c.MetadataMode) {
		return errors.New("invalid metadata mode")
	}

	return nil
}

var _ Batcher = (*TokenCountBatcher)(nil)

// TokenCountBatcher splits documents into optimized batches based on token count limits.
//
// This batcher is useful for:
//   - Optimizing API calls to embedding services with token limits
//   - Preventing request failures due to exceeding maximum token counts
//   - Balancing batch sizes for efficient parallel processing
//   - Ensuring document order is preserved for correct embedding-to-document mapping
//
// The batcher formats each document according to the configured MetadataMode,
// estimates its token count, and groups documents into batches that stay within
// the token limit. If a single document exceeds the limit, an error is returned.
type TokenCountBatcher struct {
	tokenCountEstimator tokenizer.Estimator
	maxInputTokenCount  int
	formatter           Formatter
	metadataMode        MetadataMode
}

func NewTokenCountBatcher(config *TokenCountBatcherConfig) (*TokenCountBatcher, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	actualMaxTokens := int(math.Round(float64(config.MaxInputTokenCount) * (1 - config.ReservePercentage)))

	return &TokenCountBatcher{
		tokenCountEstimator: config.TokenCountEstimator,
		maxInputTokenCount:  actualMaxTokens,
		formatter:           config.Formatter,
		metadataMode:        config.MetadataMode,
	}, nil
}

func (b *TokenCountBatcher) Batch(ctx context.Context, docs []*Document) ([][]*Document, error) {
	type docWithTokenCount struct {
		document   *Document
		tokenCount int
	}

	docsWithTokens := make([]*docWithTokenCount, 0, len(docs))
	for _, doc := range docs {
		formattedContent := doc.FormatByMetadataModeWithFormatter(b.metadataMode, b.formatter)

		estimatedTokens, err := b.tokenCountEstimator.EstimateText(ctx, formattedContent)
		if err != nil {
			return nil, err
		}

		if estimatedTokens > b.maxInputTokenCount {
			return nil, errors.New("tokens in a single document exceeds the maximum number of allowed input tokens")
		}

		docsWithTokens = append(docsWithTokens, &docWithTokenCount{
			document:   doc,
			tokenCount: estimatedTokens,
		})
	}

	var (
		currentTokenCount int
		resultBatches     [][]*Document
		currentBatch      []*Document
	)

	for _, docWithToken := range docsWithTokens {
		if currentTokenCount+docWithToken.tokenCount > b.maxInputTokenCount {
			if len(currentBatch) > 0 {
				resultBatches = append(resultBatches, currentBatch)
			}
			currentBatch = make([]*Document, 0)
			currentTokenCount = 0
		}

		currentBatch = append(currentBatch, docWithToken.document)
		currentTokenCount += docWithToken.tokenCount
	}

	if len(currentBatch) > 0 {
		resultBatches = append(resultBatches, currentBatch)
	}

	return resultBatches, nil
}
