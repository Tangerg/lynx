package documentpipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/tokenizer"
)

const defaultBatcherMaxTokens = 8191

// TokenCountBatcherConfig configures token estimation and the per-batch
// provider budget.
type TokenCountBatcherConfig struct {
	// Estimator is required.
	Estimator tokenizer.TextEstimator
	// MaxTokens is the provider input limit. Zero uses 8191.
	MaxTokens int
	// Reserve is the fraction of MaxTokens held back from each batch. Zero
	// means no reserve.
	Reserve float64
	// Formatter renders each document before estimation. Nil uses document
	// text without metadata.
	Formatter Formatter
	// Mode is passed to Formatter. The zero value is MetadataModeAll.
	Mode MetadataMode
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
	estimator tokenizer.TextEstimator
	maxTokens int
	formatter Formatter
	mode      MetadataMode
}

func NewTokenCountBatcher(config TokenCountBatcherConfig) (*TokenCountBatcher, error) {
	if config.Estimator == nil {
		return nil, errors.New("documentpipeline.TokenCountBatcherConfig: Estimator is required")
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = defaultBatcherMaxTokens
	}
	if config.MaxTokens < 0 {
		return nil, errors.New("documentpipeline.TokenCountBatcherConfig: MaxTokens must be > 0")
	}
	if config.Reserve < 0 || config.Reserve >= 1 {
		return nil, errors.New("documentpipeline.TokenCountBatcherConfig: Reserve must be in [0, 1)")
	}
	if config.Formatter == nil {
		config.Formatter = FormatterFunc(formatText)
	}
	if config.Mode == "" {
		config.Mode = MetadataModeAll
	}
	if !validMetadataMode(config.Mode) {
		return nil, fmt.Errorf("documentpipeline.TokenCountBatcherConfig: invalid Mode %q", config.Mode)
	}

	effective := max(1, int(float64(config.MaxTokens)*(1-config.Reserve)))
	return &TokenCountBatcher{
		estimator: config.Estimator,
		maxTokens: effective,
		formatter: config.Formatter,
		mode:      config.Mode,
	}, nil
}

func (b *TokenCountBatcher) Batch(ctx context.Context, docs []*document.Document) ([][]*document.Document, error) {
	type sized struct {
		doc    *document.Document
		tokens int
	}

	scored := make([]sized, 0, len(docs))
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rendered, err := b.formatter.Format(doc, b.mode)
		if err != nil {
			return nil, err
		}

		count, err := b.estimator.EstimateText(ctx, rendered)
		if err != nil {
			return nil, err
		}
		if count > b.maxTokens {
			return nil, fmt.Errorf("documentpipeline.TokenCountBatcher.Batch: document %q has %d tokens, exceeds per-batch budget %d",
				doc.ID, count, b.maxTokens)
		}
		scored = append(scored, sized{doc: doc, tokens: count})
	}

	var (
		batches      [][]*document.Document
		currentBatch []*document.Document
		currentSum   int
	)

	for _, item := range scored {
		if currentSum+item.tokens > b.maxTokens {
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
			}
			currentBatch = nil
			currentSum = 0
		}
		currentBatch = append(currentBatch, item.doc)
		currentSum += item.tokens
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}
	return batches, nil
}
