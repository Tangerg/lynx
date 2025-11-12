package flow

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

// BatchConfig contains the configuration for creating a Batch node.
// It defines how to split input into segments, process them, and combine results.
//
// Type parameters:
//   - I: Input type for the batch operation
//   - O: Output type after aggregation
//   - T: Type of individual segments
//   - R: Type of results from processing each segment
type BatchConfig[I any, O any, T any, R any] struct {
	// Node is the processing unit to be executed for each segment
	Node Node[T, R]

	// Segmenter divides the input into multiple segments for parallel processing
	Segmenter func(context.Context, I) ([]T, error)

	// Aggregator combines all segment results into a single output
	Aggregator func(context.Context, []R) (O, error)

	// ContinueOnError determines whether to continue processing remaining segments
	// when one segment fails. If false, the batch stops on first error.
	ContinueOnError bool

	// ConcurrencyLimit sets the maximum number of concurrent segment processors.
	// If <= 0, defaults to 1 (sequential processing).
	ConcurrencyLimit int
}

// validate checks if the BatchConfig is valid and ready to use.
// Returns an error if any required field is missing or invalid.
func (cfg *BatchConfig[I, O, T, R]) validate() error {
	if cfg == nil {
		return errors.New("batch config cannot be nil")
	}

	if cfg.Node == nil {
		return errors.New("batch node cannot be nil")
	}

	if cfg.Segmenter == nil {
		return errors.New("segmenter is required: batch processing needs a function to divide input into segments")
	}

	if cfg.Aggregator == nil {
		return errors.New("aggregator is required: batch processing needs a function to combine segment results")
	}

	return nil
}

// Batch represents a node that processes input in parallel segments.
// It splits input into multiple parts, processes them concurrently,
// and aggregates the results into a single output.
//
// Type parameters:
//   - I: Input type for the batch operation
//   - O: Output type after aggregation
//   - T: Type of individual segments
//   - R: Type of results from processing each segment
type Batch[I any, O any, T any, R any] struct {
	node             Node[T, R]
	segmenter        func(context.Context, I) ([]T, error)
	aggregator       func(context.Context, []R) (O, error)
	continueOnError  bool
	concurrencyLimit int
}

// NewBatch creates a new Batch instance with the provided configuration.
// Returns an error if the configuration is invalid.
//
// Example:
//
//	batch, err := NewBatch(&BatchConfig{
//	    Node: processorNode,
//	    Segmenter: func(ctx context.Context, input []int) ([]int, error) {
//	        return input, nil // Each element is a segment
//	    },
//	    Aggregator: func(ctx context.Context, results []int) (int, error) {
//	        sum := 0
//	        for _, r := range results {
//	            sum += r
//	        }
//	        return sum, nil
//	    },
//	    ConcurrencyLimit: 10,
//	})
func NewBatch[I any, O any, T any, R any](cfg *BatchConfig[I, O, T, R]) (*Batch[I, O, T, R], error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Batch[I, O, T, R]{
		node:             cfg.Node,
		segmenter:        cfg.Segmenter,
		aggregator:       cfg.Aggregator,
		continueOnError:  cfg.ContinueOnError,
		concurrencyLimit: cfg.ConcurrencyLimit,
	}, nil
}

// getConcurrencyLimit returns the effective concurrency limit.
// Returns 1 (sequential processing) if the configured limit is invalid.
func (b *Batch[I, O, T, R]) getConcurrencyLimit() int {
	if b.concurrencyLimit <= 0 {
		return 1
	}
	return b.concurrencyLimit
}

// runSequential processes all segments sequentially (one at a time).
// Returns collected results and any error encountered.
// If continueOnError is true, failed segments are skipped and processing continues.
func (b *Batch[I, O, T, R]) runSequential(ctx context.Context, segments []T) ([]R, error) {
	results := make([]R, 0, len(segments))

	for _, segment := range segments {
		result, err := b.node.Run(ctx, segment)

		if err == nil {
			results = append(results, result)
		} else if !b.continueOnError {
			return nil, err
		}
		// If continueOnError is true and error occurred, skip this result
	}

	return results, nil
}

// runConcurrent processes segments concurrently with the configured limit.
// Preserves the original order of segments in the results.
// Returns collected results and any error encountered.
func (b *Batch[I, O, T, R]) runConcurrent(ctx context.Context, segments []T) ([]R, error) {
	// order preserves the original segment order in the results
	order := make([]*R, len(segments))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(b.getConcurrencyLimit())

	for i, segment := range segments {
		group.Go(func() error {
			result, err := b.node.Run(groupCtx, segment)

			if err == nil {
				order[i] = &result
				return nil
			}

			if !b.continueOnError {
				return err
			}

			// If continueOnError is true, don't propagate error
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	// Collect non-nil results, preserving order
	results := make([]R, 0, len(segments))
	for _, r := range order {
		if r != nil {
			results = append(results, *r)
		}
	}

	return results, nil
}

// Run implements the Node interface for Batch.
// It splits the input into segments, processes them (sequentially or concurrently),
// and aggregates the results into a single output.
//
// Processing steps:
//  1. Split input into segments using the segmenter
//  2. Process segments (sequentially if limit=1, otherwise concurrently)
//  3. Aggregate results using the aggregator
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Input value to be split and processed
//
// Returns:
//   - The aggregated output from all successful segment results
//   - An error if segmentation, processing, or aggregation fails
func (b *Batch[I, O, T, R]) Run(ctx context.Context, input I) (O, error) {
	var output O

	// Step 1: Split input into segments
	segments, err := b.segmenter(ctx, input)
	if err != nil {
		return output, err
	}

	// Step 2: Process segments
	var results []R
	if b.getConcurrencyLimit() == 1 {
		results, err = b.runSequential(ctx, segments)
	} else {
		results, err = b.runConcurrent(ctx, segments)
	}

	if err != nil {
		return output, err
	}

	// Step 3: Aggregate results
	return b.aggregator(ctx, results)
}
