package flow

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"golang.org/x/sync/errgroup"
)

// ParallelConfig defines the configuration for a parallel node that executes
// multiple processors concurrently with the same input.
type ParallelConfig[I, O any] struct {
	// Processors is the list of functions to execute in parallel.
	// Each processor receives the same input and produces an independent output.
	Processors []func(context.Context, I) (O, error)

	// ContinueOnError determines whether to continue executing remaining processors
	// when an error occurs. If false, all processors are cancelled at the first error.
	ContinueOnError bool

	// ConcurrencyLimit controls the maximum number of concurrent processors.
	// - 0 or negative: unlimited concurrency (equal to number of processors)
	// - positive: concurrent processing with specified limit
	ConcurrencyLimit int
}

// validate checks if the parallel configuration is valid.
func (cfg *ParallelConfig[I, O]) validate() error {
	if cfg == nil {
		return errors.New("parallel config cannot be nil")
	}

	if len(cfg.Processors) == 0 {
		return errors.New("at least one processor is required")
	}

	return nil
}

var _ Node[any, []Result[any]] = (*Parallel[any, any])(nil)

// Parallel represents a node that executes multiple processors concurrently,
// all receiving the same input and producing independent outputs.
type Parallel[I, O any] struct {
	processors       []func(context.Context, I) (O, error)
	continueOnError  bool
	concurrencyLimit int
}

// NewParallel creates a new parallel node with the provided configuration.
// Returns an error if the configuration is invalid.
func NewParallel[I, O any](cfg ParallelConfig[I, O]) (*Parallel[I, O], error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid parallel config: %w", err)
	}

	return &Parallel[I, O]{
		processors:       slices.Clone(cfg.Processors),
		continueOnError:  cfg.ContinueOnError,
		concurrencyLimit: cfg.ConcurrencyLimit,
	}, nil
}

// calcConcurrencyLimit determines the actual concurrency level based on configuration
// and number of processors.
func (p *Parallel[I, O]) calcConcurrencyLimit() int {
	if p.concurrencyLimit <= 0 {
		return len(p.processors)
	}

	return min(p.concurrencyLimit, len(p.processors))
}

// Run executes all processors in parallel with the same input.
// Returns a slice of results corresponding to each processor.
//
// If ContinueOnError is false, execution stops at the first error and all
// remaining processors are cancelled.
func (p *Parallel[I, O]) Run(ctx context.Context, input I) ([]Result[O], error) {
	results := make([]Result[O], len(p.processors))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(p.calcConcurrencyLimit())

	for index, processor := range p.processors {
		// Capture loop variables for goroutine
		idx := index
		proc := processor

		group.Go(func() error {
			result, err := proc(groupCtx, input)

			// Stop all processing if error occurs and continueOnError is false
			if err != nil && !p.continueOnError {
				return fmt.Errorf("processor %d failed: %w", idx, err)
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
