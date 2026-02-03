package flow

import (
	"context"
	"errors"
	"fmt"
)

// LoopConfig defines the configuration for a loop node that repeatedly processes data
// until a termination condition is met.
type LoopConfig[T any] struct {
	// Processor is the function executed in each iteration.
	// It receives the iteration index and current value, returning:
	//   - output: the transformed value for the next iteration
	//   - done: whether to terminate the loop
	//   - err: any error that occurred during processing
	Processor func(ctx context.Context, iteration int, input T) (output T, done bool, err error)

	// MaxIterations limits the number of loop iterations.
	//
	// Values:
	//   - 0: Uses DefaultMaxIterations (1024)
	//   - Positive integer: Custom iteration limit
	//   - Must not be negative
	//
	// The loop will return an error if this limit is reached without
	// the processor returning done=true.
	MaxIterations int
}

// validate checks if the loop configuration is valid.
func (cfg *LoopConfig[T]) validate() error {
	if cfg == nil {
		return errors.New("loop config cannot be nil")
	}

	if cfg.Processor == nil {
		return errors.New("loop processor cannot be nil")
	}
	if cfg.MaxIterations < 0 {
		return errors.New("max iterations must be greater than zero")
	}
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 1024
	}
	return nil
}

var _ Node[any, any] = (*Loop[any])(nil)

// Loop represents a node that repeatedly processes data until a condition is met.
type Loop[T any] struct {
	processor     func(context.Context, int, T) (T, bool, error)
	maxIterations int
}

// NewLoop creates a new loop node with the provided configuration.
// Returns an error if the configuration is invalid.
func NewLoop[T any](cfg LoopConfig[T]) (*Loop[T], error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid loop config: %w", err)
	}

	return &Loop[T]{
		processor:     cfg.Processor,
		maxIterations: cfg.MaxIterations,
	}, nil
}

// Run executes the loop, repeatedly applying the processor until completion or error.
// The loop terminates when:
//   - The processor returns done=true
//   - An error occurs
//   - MaxIterations is reached (if set)
func (l *Loop[T]) Run(ctx context.Context, input T) (T, error) {
	var (
		iteration int
		current   = input
		done      bool
		err       error
	)

	for iteration < l.maxIterations {
		// Execute processor for current iteration
		current, done, err = l.processor(ctx, iteration, current)
		if err != nil {
			return current, fmt.Errorf("loop failed at iteration %d: %w", iteration, err)
		}

		// Check termination condition
		if done {
			return current, nil
		}

		iteration++
	}

	// Exceeded max iterations
	return current, fmt.Errorf(
		"loop exceeded max iterations (%d): termination condition not met",
		l.maxIterations,
	)
}
