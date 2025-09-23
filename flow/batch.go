package flow

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

// Batch enables processing of multiple items and aggregating the results.
// Generic parameters:
// - I: Input type for the batch
// - O: Output type after aggregation
// - T: Type of each segment after dividing the input
// - R: Output type after processing each segment
type Batch[I any, O any, T any, R any] struct {
	// processor handles each individual segment
	processor Processor[T, R]
	// continueOnError determines whether to continue processing segments after an error
	continueOnError bool
	// concurrencyLimit controls the maximum number of segments processed concurrently
	concurrencyLimit int
	// segmenter divides the input into multiple segments for processing
	segmenter func(context.Context, I) ([]T, error)
	// aggregator combines the results from processing multiple segments
	aggregator func(context.Context, []R) (O, error)
}

// validate ensures that the batch has all required components.
// Returns an error if processor, segmenter, or aggregator is missing.
func (b *Batch[I, O, T, R]) validate() error {
	err := validateProcessor(b.processor)
	if err != nil {
		return err
	}
	if b.segmenter == nil {
		return errors.New("segmenter is required: batch processing needs a function to divide input into segments")
	}
	if b.aggregator == nil {
		return errors.New("aggregator is required: batch processing needs a function to combine segment results")
	}
	return nil
}

// createSegments divides the input into multiple segments for processing.
// It first checks for context cancellation, then calls the segmenter function.
// Returns the segments and any error encountered during segmentation.
func (b *Batch[I, O, T, R]) createSegments(ctx context.Context, input I) ([]T, error) {
	err := b.processor.checkContextCancellation(ctx)
	if err != nil {
		return nil, err
	}
	return b.segmenter(ctx, input)
}

// aggregateResults combines the results from processing multiple segments.
// It first checks for context cancellation, then calls the aggregator function.
// Returns the aggregated result and any error encountered during aggregation.
func (b *Batch[I, O, T, R]) aggregateResults(ctx context.Context, results []R) (res O, err error) {
	err = b.processor.checkContextCancellation(ctx)
	if err != nil {
		return
	}
	return b.aggregator(ctx, results)
}

// getConcurrencyLimit returns the concurrency limit, defaulting to 1 if not set.
// A value of 1 means sequential processing, while higher values enable concurrent processing.
func (b *Batch[I, O, T, R]) getConcurrencyLimit() int {
	if b.concurrencyLimit == 0 {
		return 1
	}
	return b.concurrencyLimit
}

// runOne processes segments sequentially (when concurrency limit is 1).
// It processes each segment in order and collects the results.
// If continueOnError is false, it stops on the first error.
// Returns the collected results and any error encountered during processing.
func (b *Batch[I, O, T, R]) runOne(ctx context.Context, segments []T) ([]R, error) {
	var results []R
	for _, segment := range segments {
		res, err := b.processor.Run(ctx, segment)
		if err == nil {
			results = append(results, res)
		} else if !b.continueOnError {
			return nil, err
		}
	}
	return results, nil
}

// runN processes segments concurrently with a specified concurrency limit.
// It uses errgroup to manage goroutines and maintain the original segment order in results.
// If continueOnError is false, it stops all processing on the first error.
// Returns the collected results and any error encountered during processing.
func (b *Batch[I, O, T, R]) runN(ctx context.Context, segments []T) ([]R, error) {
	var (
		// order preserves the original segment order in the results
		order           = make([]*R, len(segments))
		group, groupCtx = errgroup.WithContext(ctx)
	)
	group.SetLimit(b.getConcurrencyLimit())
	for i, segment := range segments {
		group.Go(func() error {
			res, err := b.processor.Run(groupCtx, segment)
			if err == nil {
				order[i] = &res
			}
			if !b.continueOnError {
				return err
			}
			return nil
		})
	}
	err := group.Wait()
	if err != nil {
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

// run executes the batch processing logic.
// It segments the input, processes each segment (sequentially or concurrently),
// and aggregates the results.
// Returns the aggregated output and any error encountered during processing.
func (b *Batch[I, O, T, R]) run(ctx context.Context, input I) (output O, err error) {
	segments, err := b.createSegments(ctx, input)
	if err != nil {
		return
	}
	var results []R
	if b.getConcurrencyLimit() == 1 {
		results, err = b.runOne(ctx, segments)
	} else {
		results, err = b.runN(ctx, segments)
	}
	if err != nil {
		return
	}
	return b.aggregateResults(ctx, results)
}

// Run implements the Node interface for Batch.
// It first validates the batch configuration, then executes the batch processing logic.
func (b *Batch[I, O, T, R]) Run(ctx context.Context, input I) (output O, err error) {
	err = b.validate()
	if err != nil {
		return
	}
	return b.run(ctx, input)
}

// WithContinueOnError sets whether to continue processing segments after an error.
// If true, processing continues with remaining segments when one fails.
// If false (default), processing stops on the first error.
// Returns the Batch for chaining.
func (b *Batch[I, O, T, R]) WithContinueOnError() *Batch[I, O, T, R] {
	b.continueOnError = true
	return b
}

// WithConcurrencyLimit sets the maximum number of segments to process concurrently.
// A value of 0 or 1 means sequential processing, while higher values enable concurrent processing.
// Returns the Batch for chaining.
func (b *Batch[I, O, T, R]) WithConcurrencyLimit(concurrencyLimit int) *Batch[I, O, T, R] {
	b.concurrencyLimit = concurrencyLimit
	return b
}

// WithProcessor sets the processor function for handling each segment.
// Returns the Batch for chaining.
func (b *Batch[I, O, T, R]) WithProcessor(processor Processor[T, R]) *Batch[I, O, T, R] {
	b.processor = processor
	return b
}

// WithSegmenter sets the function that divides the input into segments.
// Returns the Batch for chaining.
func (b *Batch[I, O, T, R]) WithSegmenter(segmenter func(context.Context, I) ([]T, error)) *Batch[I, O, T, R] {
	b.segmenter = segmenter
	return b
}

// WithAggregator sets the function that combines segment results into a final output.
// Returns the Batch for chaining.
func (b *Batch[I, O, T, R]) WithAggregator(aggregator func(context.Context, []R) (O, error)) *Batch[I, O, T, R] {
	b.aggregator = aggregator
	return b
}

// BatchBuilder helps construct a Batch node with a fluent API.
// It maintains references to both the parent flow and the batch being built.
type BatchBuilder struct {
	flow  *Flow
	batch *Batch[any, any, any, any]
}

// WithContinueOnError sets whether to continue processing segments after an error.
// Returns the BatchBuilder for chaining.
func (b *BatchBuilder) WithContinueOnError() *BatchBuilder {
	b.batch.WithContinueOnError()
	return b
}

// WithConcurrencyLimit sets the maximum number of segments to process concurrently.
// Returns the BatchBuilder for chaining.
func (b *BatchBuilder) WithConcurrencyLimit(concurrencyLimit int) *BatchBuilder {
	b.batch.WithConcurrencyLimit(concurrencyLimit)
	return b
}

// WithProcessor sets the processor function for handling each segment.
// Returns the BatchBuilder for chaining.
func (b *BatchBuilder) WithProcessor(processor Processor[any, any]) *BatchBuilder {
	b.batch.WithProcessor(processor)
	return b
}

// WithSegmenter sets the function that divides the input into segments.
// Returns the BatchBuilder for chaining.
func (b *BatchBuilder) WithSegmenter(segmenter func(context.Context, any) ([]any, error)) *BatchBuilder {
	b.batch.WithSegmenter(segmenter)
	return b
}

// WithAggregator sets the function that combines segment results into a final output.
// Returns the BatchBuilder for chaining.
func (b *BatchBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *BatchBuilder {
	b.batch.WithAggregator(aggregator)
	return b
}

// Then adds the constructed batch to the parent flow and returns the flow.
// This completes the batch construction and continues building the flow.
func (b *BatchBuilder) Then() *Flow {
	b.flow.Then(b.batch)
	return b.flow
}
