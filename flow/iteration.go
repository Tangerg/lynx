package flow

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// IterationConfig defines the configuration for an iteration node that applies
// a processor function to each element in a collection.
type IterationConfig[I, O any] struct {
	// Processor is the function applied to each element.
	// It receives the element's index and value, returning the transformed output.
	Processor func(context.Context, int, I) (O, error)

	// ContinueOnError determines whether to continue processing remaining elements
	// when an error occurs. If false, the iteration stops at the first error.
	ContinueOnError bool

	// ConcurrencyLimit controls the maximum number of concurrent processors.
	// - 0: sequential processing (default if not set)
	// - 1: sequential processing
	// - >1: concurrent processing with specified limit
	// - <0: unlimited concurrency (equal to slice length)
	ConcurrencyLimit int
}

// validate checks if the iteration configuration is valid and applies defaults.
func (cfg *IterationConfig[I, O]) validate() error {
	if cfg == nil {
		return errors.New("iteration config cannot be nil")
	}

	if cfg.Processor == nil {
		return errors.New("processor cannot be nil")
	}

	// Set default concurrency limit to sequential processing
	if cfg.ConcurrencyLimit == 0 {
		cfg.ConcurrencyLimit = 1
	}

	return nil
}

var _ Node[[]any, []Result[any]] = (*Iteration[any, any])(nil)

// Iteration represents a node that applies a processor function to each element
// in a collection, either sequentially or concurrently.
type Iteration[I, O any] struct {
	processor        func(context.Context, int, I) (O, error)
	continueOnError  bool
	concurrencyLimit int
}

// NewIteration creates a new iteration node with the provided configuration.
// Returns an error if the configuration is invalid.
func NewIteration[I, O any](cfg IterationConfig[I, O]) (*Iteration[I, O], error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid iteration config: %w", err)
	}

	return &Iteration[I, O]{
		processor:        cfg.Processor,
		continueOnError:  cfg.ContinueOnError,
		concurrencyLimit: cfg.ConcurrencyLimit,
	}, nil
}

// calcConcurrencyLimit determines the actual concurrency level based on configuration
// and input size.
func (it *Iteration[I, O]) calcConcurrencyLimit(elements []I) int {
	if it.concurrencyLimit < 0 {
		return len(elements)
	}

	if it.concurrencyLimit == 0 {
		return 1
	}

	return min(it.concurrencyLimit, len(elements))
}

// runSequential processes elements one by one in order.
func (it *Iteration[I, O]) runSequential(ctx context.Context, elements []I) ([]Result[O], error) {
	results := make([]Result[O], len(elements))

	for index, element := range elements {
		result, err := it.processor(ctx, index, element)

		// Stop processing if error occurs and continueOnError is false
		if err != nil && !it.continueOnError {
			return nil, fmt.Errorf("iteration failed at index %d: %w", index, err)
		}

		// Store result (with or without error)
		results[index] = Result[O]{
			Value: result,
			Error: err,
		}
	}

	return results, nil
}

// runConcurrent processes elements concurrently with specified concurrency limit.
func (it *Iteration[I, O]) runConcurrent(ctx context.Context, elements []I, concurrency int) ([]Result[O], error) {
	results := make([]Result[O], len(elements))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(concurrency)

	for index, element := range elements {
		// Capture loop variables for goroutine
		idx := index
		elem := element

		group.Go(func() error {
			result, err := it.processor(groupCtx, idx, elem)

			// Stop all processing if error occurs and continueOnError is false
			if err != nil && !it.continueOnError {
				return fmt.Errorf("iteration failed at index %d: %w", idx, err)
			}

			// Store result (with or without error)
			results[idx] = Result[O]{
				Value: result,
				Error: err,
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// Run executes the iteration, processing each element in the input slice.
// Returns a slice of results corresponding to each input element.
//
// The execution strategy (sequential vs concurrent) is determined by the
// ConcurrencyLimit configuration.
func (it *Iteration[I, O]) Run(ctx context.Context, input []I) ([]Result[O], error) {
	concurrency := it.calcConcurrencyLimit(input)

	if concurrency == 1 {
		return it.runSequential(ctx, input)
	}

	return it.runConcurrent(ctx, input, concurrency)
}
