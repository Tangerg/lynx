// Package function provides an interface for defining callable functions
// within a system, including metadata and execution capabilities.

package function

import (
	"context"
)

// Function defines the contract for a callable function in a system,
// encapsulating metadata and execution logic. It is designed to facilitate
// the retrieval of function details and execution with input data.
//
// Methods:
//
// Name:
//
//	Name() string
//	Retrieves the name of the function. This identifier can be used for
//	logging, debugging, or selection purposes in a system.
//	Returns:
//	- string: The name of the function.
//
// Description:
//
//	Description() string
//	Provides a textual description of the function's purpose or behavior.
//	This is particularly useful for documentation, user interfaces, or
//	developer reference.
//	Returns:
//	- string: The description of the function.
//
// InputTypeSchema:
//
//	InputTypeSchema() string
//	Describes the expected input type schema for the function. This can be
//	used for validation, tooling, or guiding users to provide properly
//	formatted input.
//	Returns:
//	- string: The schema of the input type expected by the function.
//
// Call:
//
//	Call(ctx context.Context, input string) (string, error)
//	Executes the function with the provided input and context. The context
//	allows for cancellation, timeouts, and request-scoped values.
//
//	Parameters:
//	- ctx: A context.Context instance to manage the request lifecycle.
//	- input: A string representing the input data for the function.
//
//	Returns:
//	- string: The result of the function execution.
//	- error: An error if the execution fails, providing details on the failure.
//
// Example Implementation:
//
//	type MyFunction struct{}
//
//	func (f MyFunction) Name() string {
//	    return "ExampleFunction"
//	}
//
//	func (f MyFunction) Description() string {
//	    return "This function processes input and returns a result."
//	}
//
//	func (f MyFunction) InputTypeSchema() string {
//	    return `{"type": "string"}`
//	}
//
//	func (f MyFunction) Call(ctx context.Context, input string) (string, error) {
//	    if input == "" {
//	        return "", errors.New("input cannot be empty")
//	    }
//	    return "Processed: " + input, nil
//	}
type Function interface {
	// Name returns the name of the function.
	Name() string

	// Description returns a textual explanation of the function's purpose or behavior.
	Description() string

	// InputTypeSchema returns the schema of the input type expected by the function.
	InputTypeSchema() string

	// Call executes the function with the provided input and context.
	// It returns the result of the execution as a string and an error if the
	// execution fails.
	Call(ctx context.Context, input string) (string, error)
}
