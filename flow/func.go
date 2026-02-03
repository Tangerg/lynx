package flow

import (
	"context"
	"errors"
)

var _ Node[any, any] = (*Func[any, any])(nil)

// Func is a function type that implements the Node interface, allowing regular functions
// to be used as nodes in a workflow pipeline.
//
// This adapter pattern enables seamless integration of functions into the flow system
// without requiring explicit struct definitions.
//
// Generic parameters:
//   - I: Input type
//   - O: Output type
//
// Example:
//
//	multiply := Func[int, int](func(ctxcontext.Context,xint) (int, error) {
//	    return x * 2, nil
//	})
//	result, err := multiply.Run(ctx, 5) // Returns 10
type Func[I, O any] func(ctx context.Context, input I) (output O, err error)

// Run executes the function with the provided context and input.
// Returns an error if the function is nil.
func (f Func[I, O]) Run(ctx context.Context, input I) (output O, err error) {
	if f == nil {
		var zero O
		return zero, errors.New("cannot run nil function: func is not initialized")
	}

	return f(ctx, input)
}
