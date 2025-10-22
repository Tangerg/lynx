package batchers

import (
	"context"
	"errors"
	"math"
	"slices"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

type TokenCountBatcherConfig struct {
	TokenCountEstimator tokenizer.Estimator
	MaxInputTokenCount  int
	ReservePercentage   float64
	Formatter           document.Formatter
	MetadataMode        document.MetadataMode
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

var _ document.Batcher = (*TokenCountBatcher)(nil)

type TokenCountBatcher struct {
	tokenCountEstimator tokenizer.Estimator
	maxInputTokenCount  int
	formatter           document.Formatter
	metadataMode        document.MetadataMode
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

func (b *TokenCountBatcher) Batch(ctx context.Context, docs []*document.Document) ([][]*document.Document, error) {
	type docWithTokenCount struct {
		document   *document.Document
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
		resultBatches     [][]*document.Document
		currentBatch      []*document.Document
	)

	for _, docWithToken := range docsWithTokens {
		if currentTokenCount+docWithToken.tokenCount > b.maxInputTokenCount {
			if len(currentBatch) > 0 {
				resultBatches = append(resultBatches, currentBatch)
			}
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
