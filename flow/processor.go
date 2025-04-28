// Package flow provides a robust, composable pipeline framework for creating data processing workflows.
package flow

import "context"

// Processor represents a function that transforms input data into output data.
//
// The Processor type encapsulates the core processing logic that each node in a flow
// pipeline executes. It takes an input value of type I and a context for cancellation
// support, and returns an output value of type O or an error.
//
// Processor is the fundamental building block for data transformation in the flow
// framework. By defining processing logic as a first-class type, the framework
// enables flexible composition and reuse of processing functions.
//
// Example:
//
//	// Define a processor that converts strings to uppercase
//	uppercase := Processor[string, string](func(ctx context.Context,input string) (string, error) {
//		return strings.ToUpper(input), nil
//	})
type Processor[I any, O any] func(context.Context, I) (O, error)

// AsProcessor converts a regular function to a Processor type.
//
// This utility function allows regular functions that match the Processor signature
// to be explicitly converted to the Processor type. This is useful when passing
// functions to methods that expect a Processor parameter.
//
// The conversion is type-safe and preserves the input and output types of the
// original function.
//
// Example:
//
//	// Convert a regular function to a Processor
//	validateData := flow.AsProcessor(func(ctx context.Context, data Record) (ValidatedRecord, error) {
//		// Validation logic
//		return validated, nil
//	})
func AsProcessor[I any, O any](fn func(context.Context, I) (O, error)) Processor[I, O] {
	return fn
}

// Middleware represents a function that wraps a processor with additional behavior.
//
// Middleware functions take a processor and return a new processor that adds
// functionality before, after, or around the original processing logic. This
// allows for cross-cutting concerns like logging, metrics, validation, or error
// handling to be applied consistently across multiple processors.
//
// Middleware can be chained together to build up complex behaviors from simple,
// reusable components.
//
// Example:
//
//	// Create a logging middleware
//	loggingMiddleware := flow.Middleware[any, any](func(p flow.Processor[any,any]) flow.Processor[any, any] {
//		return func(ctx context.Context, input any) (any, error) {
//			log.Printf("Processing input: %v", input)
//			output, err := p(ctx, input)
//			log.Printf("Processing result: %v, error: %v", output, err)
//			return output, err
//		}
//	})
//
//	// Apply middleware to a processor
//	processWithLogging := loggingMiddleware(myProcessor)
type Middleware[I any, O any] func(processor Processor[I, O]) Processor[I, O]
