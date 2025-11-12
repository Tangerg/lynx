package flow

import (
	"context"
	"errors"
)

// LoopConfig contains the configuration for creating a Loop node.
// It defines the node to be executed repeatedly and the termination condition.
type LoopConfig[I any, O any] struct {
	// Node is the processing unit to be executed in each iteration
	Node Node[I, O]

	// MaxIterations sets the hard limit for the number of loop iterations.
	// This prevents infinite loops and runaway executions.
	// If <= 0, no limit is enforced (loop relies solely on Terminator).
	// The iteration count is 0-based, so MaxIterations=10 means iterations 0-9.
	// This limit is checked before evaluating the Terminator condition.
	MaxIterations int

	// Terminator is an optional function that determines when to stop the loop.
	// Parameters:
	//   - ctx: Context for cancellation control
	//   - iteration: Current iteration count (0-based)
	//   - input: Original input to the loop
	//   - output: Output from the current iteration
	// Returns:
	//   - bool: true to terminate the loop, false to continue
	//   - error: Any error that occurred during termination check
	// If nil, the loop executes only once.
	Terminator func(context.Context, int, I, O) (bool, error)
}

// validate checks if the LoopConfig is valid and ready to use.
// Returns an error if the config or its Node field is nil.
func (cfg *LoopConfig[I, O]) validate() error {
	if cfg == nil {
		return errors.New("loop config cannot be nil")
	}

	if cfg.Node == nil {
		return errors.New("loop node cannot be nil")
	}

	return nil
}

// Loop represents a node that executes another node repeatedly until a termination condition is met.
// The output of each iteration can be used to determine if the loop should continue.
type Loop[I any, O any] struct {
	node          Node[I, O]
	maxIterations int
	terminator    func(context.Context, int, I, O) (bool, error)
}

// NewLoop creates a new Loop instance with the provided configuration.
// Returns an error if the configuration is invalid.
//
// Example:
//
//	loop, err := NewLoop(&LoopConfig{
//	    Node: myNode,
//	    Terminator: func(ctx context.Context, iteration int, input I, output O) (bool, error) {
//	        return iteration >= 10, nil // Stop after 10 iterations
//	    },
//	})
func NewLoop[I any, O any](cfg *LoopConfig[I, O]) (*Loop[I, O], error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Loop[I, O]{
		node:          cfg.Node,
		maxIterations: cfg.MaxIterations,
		terminator:    cfg.Terminator,
	}, nil
}

// shouldTerminate determines whether the loop should stop iterating.
//
// Termination rules:
//   - MaxIterations + Terminator: stops when any one conditions are satisfied (OR logic)
//   - MaxIterations only: stops when iteration reaches the limit (iteration >= MaxIterations-1)
//   - Terminator only: stops when terminator returns true
//   - Neither set: executes once and stops
//
// Parameters:
//   - ctx: Context for cancellation control
//   - iteration: Current iteration count (0-based)
//   - input: Original input to the loop
//   - output: Output from the current iteration
//
// Returns:
//   - bool: true to stop, false to continue
//   - error: Any error from the terminator function
func (l *Loop[I, O]) shouldTerminate(ctx context.Context, iteration int, input I, output O) (bool, error) {
	// Case 1: Both limits set - require both conditions
	if l.maxIterations > 0 && l.terminator != nil {
		terminator, err := l.terminator(ctx, iteration, input, output)
		if err != nil {
			return false, err
		}
		return (iteration >= l.maxIterations-1) || terminator, nil
	}

	// Case 2: Only max iterations set
	if l.maxIterations > 0 {
		return iteration >= l.maxIterations-1, nil
	}

	// Case 3: No termination condition - single iteration
	if l.terminator == nil {
		return true, nil
	}

	// Case 4: Only terminator set
	return l.terminator(ctx, iteration, input, output)
}

// Run implements the Node interface for Loop.
// It repeatedly executes the configured node until the termination condition is met.
// Each iteration uses the same input, but the output is updated after each execution.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Input value for the loop node
//
// Returns:
//   - The output from the final iteration
//   - An error if any iteration fails or the terminator returns an error
func (l *Loop[I, O]) Run(ctx context.Context, input I) (O, error) {
	var iteration int

	for {
		output, err := l.node.Run(ctx, input)
		if err != nil {
			return output, err
		}

		shouldStop, err := l.shouldTerminate(ctx, iteration, input, output)
		if err != nil {
			return output, err
		}

		if shouldStop {
			return output, nil
		}

		iteration++
	}
}
