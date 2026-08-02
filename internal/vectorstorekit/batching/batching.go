// Package batching enforces the vector-store batching contract at adapter
// boundaries.
package batching

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
)

// ErrInvalidOutput identifies a Batcher result that is not an order-
// preserving partition of its input.
var ErrInvalidOutput = errors.New("vectorstores.batching: invalid batcher output")

// Batch delegates to batcher and validates that its output preserves every
// input document exactly once, in order, without empty batches.
func Batch(
	ctx context.Context,
	batcher vectorstore.Batcher,
	documents []*document.Document,
) ([][]*document.Document, error) {
	if batcher == nil {
		return nil, errors.New("vectorstores.batching: batcher must not be nil")
	}
	batches, err := batcher.Batch(ctx, documents)
	if err != nil {
		return nil, err
	}
	if err := validate(batches, documents); err != nil {
		return nil, err
	}
	return batches, nil
}

func validate(batches [][]*document.Document, documents []*document.Document) error {
	next := 0
	for batchIndex, batch := range batches {
		if len(batch) == 0 {
			return fmt.Errorf("%w: batch %d is empty", ErrInvalidOutput, batchIndex)
		}
		for documentIndex, doc := range batch {
			if next >= len(documents) {
				return fmt.Errorf("%w: unexpected document at batch %d index %d",
					ErrInvalidOutput, batchIndex, documentIndex)
			}
			if doc != documents[next] {
				return fmt.Errorf("%w: document at batch %d index %d does not match input index %d",
					ErrInvalidOutput, batchIndex, documentIndex, next)
			}
			next++
		}
	}
	if next != len(documents) {
		return fmt.Errorf("%w: returned %d of %d documents", ErrInvalidOutput, next, len(documents))
	}
	return nil
}
