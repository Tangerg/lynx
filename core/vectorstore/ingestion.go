package vectorstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/document"
)

// ErrInvalidBatcherOutput identifies a [Batcher] result that is not an
// order-preserving partition of its input.
var ErrInvalidBatcherOutput = errors.New("vectorstore: invalid batcher output")

// ValidateDocuments enforces the provider-independent ingestion contract
// before a batcher, embedding model, or provider client observes the input.
func ValidateDocuments(documents []*document.Document) error {
	if len(documents) == 0 {
		return ErrEmptyDocuments
	}

	seen := make(map[string]int, len(documents))
	for i, doc := range documents {
		if doc == nil {
			return fmt.Errorf("%w: documents[%d] is nil", ErrInvalidDocument, i)
		}
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("%w: documents[%d]: %w", ErrInvalidDocument, i, err)
		}
		if strings.TrimSpace(doc.ID) == "" {
			return fmt.Errorf("%w: documents[%d]", ErrMissingDocumentID, i)
		}
		if doc.Text == "" {
			return fmt.Errorf("%w: documents[%d] has no text to embed", ErrInvalidDocument, i)
		}
		if first, duplicate := seen[doc.ID]; duplicate {
			return fmt.Errorf("%w %q at documents[%d] and documents[%d]",
				ErrDuplicateDocumentID, doc.ID, first, i)
		}
		seen[doc.ID] = i
	}
	return nil
}

// BatchDocuments delegates to batcher and verifies the [Batcher] contract.
func BatchDocuments(
	ctx context.Context,
	batcher Batcher,
	documents []*document.Document,
) ([][]*document.Document, error) {
	if batcher == nil {
		return nil, errors.New("vectorstore: batcher must not be nil")
	}
	batches, err := batcher.Batch(ctx, documents)
	if err != nil {
		return nil, err
	}
	if err := validateBatches(batches, documents); err != nil {
		return nil, err
	}
	return batches, nil
}

func validateBatches(batches [][]*document.Document, documents []*document.Document) error {
	next := 0
	for batchIndex, batch := range batches {
		if len(batch) == 0 {
			return fmt.Errorf("%w: batch %d is empty", ErrInvalidBatcherOutput, batchIndex)
		}
		for documentIndex, doc := range batch {
			if next >= len(documents) {
				return fmt.Errorf("%w: unexpected document at batch %d index %d",
					ErrInvalidBatcherOutput, batchIndex, documentIndex)
			}
			if doc != documents[next] {
				return fmt.Errorf("%w: document at batch %d index %d does not match input index %d",
					ErrInvalidBatcherOutput, batchIndex, documentIndex, next)
			}
			next++
		}
	}
	if next != len(documents) {
		return fmt.Errorf("%w: returned %d of %d documents", ErrInvalidBatcherOutput, next, len(documents))
	}
	return nil
}
