package document

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/lynx/core/tokenizer"
)

const (
	defaultBatcherMaxInputTokenCount = 8191
	defaultBatcherReservePercentage  = 0.1
)

type TokenCountBatcherConfig struct {
	TokenCountEstimator tokenizer.Estimator
	MaxInputTokenCount  int
	ReservePercentage   float64
	Formatter           Formatter
	MetadataMode        MetadataMode
}

func (c *TokenCountBatcherConfig) Validate() error {
	if c.TokenCountEstimator == nil {
		return errors.New("document.TokenCountBatcherConfig: TokenCountEstimator is required")
	}
	if c.Formatter == nil {
		return errors.New("document.TokenCountBatcherConfig: Formatter is required")
	}
	if c.MaxInputTokenCount <= 0 {
		return errors.New("document.TokenCountBatcherConfig: MaxInputTokenCount must be > 0")
	}
	if c.ReservePercentage < 0 || c.ReservePercentage >= 1 {
		return errors.New("document.TokenCountBatcherConfig: ReservePercentage must be in [0, 1)")
	}

	validModes := []MetadataMode{
		MetadataModeAll, MetadataModeEmbed, MetadataModeInference, MetadataModeNone,
	}
	if !slices.Contains(validModes, c.MetadataMode) {
		return fmt.Errorf("document.TokenCountBatcherConfig: invalid MetadataMode %q", c.MetadataMode)
	}
	return nil
}

func (c *TokenCountBatcherConfig) ApplyDefaults() {
	if c.MaxInputTokenCount == 0 {
		c.MaxInputTokenCount = defaultBatcherMaxInputTokenCount
	}
	if c.ReservePercentage == 0 {
		c.ReservePercentage = defaultBatcherReservePercentage
	}
}

var _ Batcher = (*TokenCountBatcher)(nil)

// TokenCountBatcher carves a document slice into batches that fit
// downstream embedding-service token limits. Document order is
// preserved across batches so callers can map embeddings back by
// position.
//
// A single document whose token count exceeds the per-batch budget is
// rejected with an error — the caller is expected to split it first
// (see [TokenSplitter]).
type TokenCountBatcher struct {
	tokenCountEstimator tokenizer.Estimator
	maxInputTokenCount  int
	formatter           Formatter
	metadataMode        MetadataMode
}

func NewTokenCountBatcher(config TokenCountBatcherConfig) (*TokenCountBatcher, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	effective := int(math.Round(float64(config.MaxInputTokenCount) * (1 - config.ReservePercentage)))
	return &TokenCountBatcher{
		tokenCountEstimator: config.TokenCountEstimator,
		maxInputTokenCount:  effective,
		formatter:           config.Formatter,
		metadataMode:        config.MetadataMode,
	}, nil
}

func (b *TokenCountBatcher) Batch(ctx context.Context, docs []*Document) ([][]*Document, error) {
	type sized struct {
		document *Document
		tokens   int
	}

	scored := make([]sized, 0, len(docs))
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rendered := doc.FormatWith(b.metadataMode, b.formatter)

		count, err := b.tokenCountEstimator.EstimateText(ctx, rendered)
		if err != nil {
			return nil, err
		}
		if count > b.maxInputTokenCount {
			return nil, fmt.Errorf("document.TokenCountBatcher.Batch: document %q has %d tokens, exceeds per-batch budget %d",
				doc.ID, count, b.maxInputTokenCount)
		}
		scored = append(scored, sized{document: doc, tokens: count})
	}

	var (
		batches      [][]*Document
		currentBatch []*Document
		currentSum   int
	)

	for _, item := range scored {
		if currentSum+item.tokens > b.maxInputTokenCount {
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
			}
			currentBatch = nil
			currentSum = 0
		}
		currentBatch = append(currentBatch, item.document)
		currentSum += item.tokens
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}
	return batches, nil
}
