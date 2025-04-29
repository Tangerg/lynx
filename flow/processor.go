package flow

import "context"

// Processor represents a function that transforms input data into output data.
// Takes an input value of type I and a context, and returns an output value of type O or an error.
//
// Example:
//
//	uppercase := Processor[string, string](func(ctx context.Context,input string) (string, error) {
//	    return strings.ToUpper(input), nil
//	})
type Processor[I any, O any] func(context.Context, I) (O, error)

// AsProcessor converts a regular function to a Processor type.
// Preserves the input and output types of the original function.
//
// Example:
//
//	validateData := flow.AsProcessor(func(ctx context.Context, data Record) (ValidatedRecord, error) {
//	    return validated, nil
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
