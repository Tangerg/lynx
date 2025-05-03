package flow

import (
	"context"
	"errors"
)

// Processor is a function type that implements basic processing logic.
// It takes a context and input of type I, and returns output of type O and an error.
// Processors are the building blocks of more complex nodes in the workflow.
type Processor[I any, O any] func(context.Context, I) (O, error)

// checkContextCancellation verifies if the context has been canceled or reached its deadline.
// Returns context.Err() if the context is done, otherwise returns nil.
func (p Processor[I, O]) checkContextCancellation(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Run implements the Node interface for Processor type, making any Processor function usable as a Node.
// It first checks for context cancellation before executing the processor function.
func (p Processor[I, O]) Run(ctx context.Context, input I) (o O, err error) {
	err = p.checkContextCancellation(ctx)
	if err != nil {
		return
	}
	return p(ctx, input)
}

// validateProcessor ensures the processor is not nil, which would lead to a panic if executed.
// Returns an error if the processor is nil, otherwise returns nil.
func validateProcessor[I any, O any](processor Processor[I, O]) error {
	if processor == nil {
		return errors.New("processor cannot be nil")
	}
	return nil
}
