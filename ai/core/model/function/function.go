package function

import (
	"context"
)

// Function is an interface that defines the contract for a callable function
// within a system, providing metadata and execution capabilities. It includes
// methods for retrieving function details and executing the function with a given input.
//
// Methods:
//
// Name() string
//   - Returns the name of the function.
//   - This method provides access to the function's identifier, which can be used
//     for logging, debugging, or selection purposes.
//
// Description() string
//   - Returns a description of the function.
//   - This method provides a textual explanation of the function's purpose or behavior,
//     which can be useful for documentation or user interfaces.
//
// InputTypeSchema() string
//   - Returns the schema of the input type expected by the function.
//   - This method provides a description or definition of the input format, which can
//     be used for validation or to guide users in providing the correct input.
//
// Call(ctx context.Context, input string) (string, error)
//   - Executes the function with the provided input and context.
//   - `ctx` is a context.Context instance that allows for cancellation, timeouts,
//     and other request-scoped values.
//   - `input` is a string representing the input data to be processed by the function.
//   - Returns the result of the function execution as a string and an error if the
//     execution fails. The result can be used for further processing or display.
type Function interface {
	Name() string
	Description() string
	InputTypeSchema() string
	Call(ctx context.Context, input string) (string, error)
}
