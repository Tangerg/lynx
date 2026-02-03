package flow

import (
	"context"
	"errors"
	"fmt"
)

var _ Node[any, any] = (*Sequence)(nil)

// Sequence represents a node that executes multiple processors in sequential order,
// passing the output of each processor as input to the next one.
//
// This node uses dynamic typing (any), which means type safety is not enforced at
// compile time. For type-safe sequential processing, consider using Pipe2 through Pipe10.
type Sequence struct {
	processors []func(context.Context, any) (any, error)
}

// NewSequence creates a new sequence node with the provided processors.
// The processors are executed in the order they are provided.
//
// Returns an error if no processors are provided.
func NewSequence(processors ...func(context.Context, any) (any, error)) (*Sequence, error) {
	if len(processors) == 0 {
		return nil, errors.New("at least one processor is required")
	}

	return &Sequence{
		processors: processors,
	}, nil
}

// Run executes all processors in sequence, where each processor's output becomes
// the input for the next processor.
//
// Execution stops immediately if any processor returns an error.
func (s *Sequence) Run(ctx context.Context, input any) (any, error) {
	current := input

	for index, processor := range s.processors {
		result, err := processor(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("sequence failed at processor %d: %w", index, err)
		}

		current = result
	}

	return current, nil
}
