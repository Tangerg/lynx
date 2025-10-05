package batchers

import (
	"context"
	"errors"
	"math"
	"slices"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

const (
	MaxInputTokenCount                 = 8191
	DefaultTokenCountReservePercentage = 0.1
)

var _ document.Batcher = (*TokenCountBatcher)(nil)

type TokenCountBatcherConfig struct {
	TokenCountEstimator tokenizer.Estimator
	MaxInputTokenCount  int
	ReservePercentage   float64
	Formatter           document.Formatter
	MetadataMode        document.MetadataMode
}

func (c *TokenCountBatcherConfig) validate() error {
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

	validModes := []document.MetadataMode{
		document.MetadataModeAll,
		document.MetadataModeEmbed,
		document.MetadataModeInference,
		document.MetadataModeNone,
	}
	if !slices.Contains(validModes, c.MetadataMode) {
		return errors.New("invalid metadata mode")
	}

	return nil
}

func (c *TokenCountBatcherConfig) initializeDefaults() {
	if c.MaxInputTokenCount == 0 {
		c.MaxInputTokenCount = MaxInputTokenCount
	}
	if c.ReservePercentage == 0 {
		c.ReservePercentage = DefaultTokenCountReservePercentage
	}
}

type TokenCountBatcher struct {
	tokenCountEstimator tokenizer.Estimator
	maxInputTokenCount  int
	formatter           document.Formatter
	metadataMode        document.MetadataMode
}

func NewTokenCountBatcher(config *TokenCountBatcherConfig) (*TokenCountBatcher, error) {
	config.initializeDefaults()

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

func (b *TokenCountBatcher) Batch(ctx context.Context, docs []*document.Document) ([][]*document.Document, error) {
	type docWithTokenCount struct {
		document   *document.Document
		tokenCount int
	}

	var docsWithTokens []*docWithTokenCount
	for _, doc := range docs {
		formattedContent := doc.FormatByMetadataModeWithFormatter(b.metadataMode, b.formatter)

		estimatedTokens, err := b.tokenCountEstimator.EstimateText(ctx, formattedContent)
		if err != nil {
			return nil, err
		}

		if estimatedTokens > b.maxInputTokenCount {
			return nil, errors.New("document token count exceeds maximum allowed input tokens")
		}

		docsWithTokens = append(docsWithTokens, &docWithTokenCount{
			document:   doc,
			tokenCount: estimatedTokens,
		})
	}

	var (
		currentTokenCount = 0
		resultBatches     [][]*document.Document
		currentBatch      []*document.Document
	)

	for _, docWithToken := range docsWithTokens {
		if currentTokenCount+docWithToken.tokenCount > b.maxInputTokenCount {
			resultBatches = append(resultBatches, currentBatch)
			currentBatch = make([]*document.Document, 0)
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
