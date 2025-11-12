package flow

import (
	"context"
	"errors"
)

// Processor is a function type that implements the Node interface.
// It provides a convenient way to create nodes from simple functions without
// defining separate struct types.
//
// Type parameters:
//   - I: Input type for the processor function
//   - O: Output type from the processor function
//
// Example:
//
//	// Define a processor function
//	toUpper := Processor[string, string](func(ctxcontext.Context,inputstring) (string, error) {
//	    return strings.ToUpper(input), nil
//	})
//
//	// Use it as a Node
//	result, err := toUpper.Run(ctx, "hello")
type Processor[I any, O any] func(context.Context, I) (O, error)

// Run implements the Node interface for Processor.
// It executes the underlying function with the provided context and input.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Input value for the processor function
//
// Returns:
//   - The output from the processor function
//   - An error if the processor is nil or the function returns an error
func (p Processor[I, O]) Run(ctx context.Context, input I) (O, error) {
	var zero O

	if p == nil {
		return zero, errors.New("processor cannot be nil")
	}

	return p(ctx, input)
}
