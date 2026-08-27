package etl

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/tokenizer"
	"github.com/samber/lo"
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
}

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
}

type sizedDocument struct {
	document *document.Document
	tokens   int
}

func NewTokenCountBatcher(config TokenCountBatcherConfig) (*TokenCountBatcher, error) {
	if lo.IsNil(config.Estimator) {
		return nil, errors.New("etl: token estimator is required")
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = defaultBatcherMaxTokens
	}
	if config.MaxTokens < 0 {
		return nil, errors.New("etl: maximum batch tokens must not be negative")
	}
	if config.Reserve < 0 || config.Reserve >= 1 {
		return nil, errors.New("etl: token reserve must be in [0, 1)")
	}
	if config.Formatter == nil {
		config.Formatter = TextFormatter{}
	} else if lo.IsNil(config.Formatter) {
		return nil, errors.New("etl: formatter must not be a typed nil")
	}

	effective := max(1, int(float64(config.MaxTokens)*(1-config.Reserve)))
	return &TokenCountBatcher{
		estimator: config.Estimator,
		maxTokens: effective,
		formatter: config.Formatter,
	}, nil
}

func (t *TokenCountBatcher) Batch(ctx context.Context, docs []*document.Document) ([][]*document.Document, error) {
	sized, err := t.measure(ctx, docs)
	if err != nil {
		return nil, err
	}
	return t.partition(sized), nil
}

func (t *TokenCountBatcher) measure(ctx context.Context, docs []*document.Document) ([]sizedDocument, error) {
	sized := make([]sizedDocument, 0, len(docs))
	for index, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if doc == nil {
			return nil, fmt.Errorf("etl: size document %d: %w", index, ErrNilDocument)
		}
		if err := doc.Validate(); err != nil {
			return nil, fmt.Errorf("etl: size document %d: %w", index, err)
		}
		rendered, err := t.formatter.Format(doc)
		if err != nil {
			return nil, fmt.Errorf("etl: format document %d for sizing: %w", index, err)
		}

		count, err := t.estimator.EstimateText(ctx, rendered)
		if err != nil {
			return nil, fmt.Errorf("etl: estimate document %d tokens: %w", index, err)
		}
		if count < 0 {
			return nil, fmt.Errorf("etl: token estimator returned %d for document %d", count, index)
		}
		if count > t.maxTokens {
			return nil, fmt.Errorf("etl: document %q has %d tokens, exceeding the batch budget of %d",
				doc.ID, count, t.maxTokens)
		}
		sized = append(sized, sizedDocument{document: doc, tokens: count})
	}
	return sized, nil
}

func (t *TokenCountBatcher) partition(sized []sizedDocument) [][]*document.Document {
	var (
		batches      [][]*document.Document
		currentBatch []*document.Document
		currentSum   int
	)

	for _, item := range sized {
		if currentSum+item.tokens > t.maxTokens {
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
	return batches
}
