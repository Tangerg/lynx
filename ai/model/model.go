package model

import (
	"context"
	"iter"
)

// Model provides a generic API for invoking AI models. It abstracts the
// interaction with various types of AI models by handling the process of
// sending requests and receiving responses. The interface uses Go generics
// to accommodate different request and response types, enhancing flexibility
// and adaptability across different AI model implementations.
//
// The Model interface follows a simple request-response pattern where each
// call is synchronous and returns a complete response. This is suitable for
// scenarios where you need the full result before proceeding, such as:
//   - Single-turn conversations or Q&A sessions
//   - Batch processing of embeddings
//   - Image generation requests
//   - Classification or analysis tasks
//
// For streaming responses, use StreamingModel instead.
type Model[Request any, Response any] interface {
	// Call executes a request to the AI model and returns the complete response.
	// This method is synchronous and blocks until the model generates the full
	// response or an error occurs.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeouts, and request-scoped values
	//   - req: The request object to send to the AI model
	//
	// Returns:
	//   - Response: The complete response from the AI model
	//   - error: Any error that occurred during model invocation
	//
	// The context enables:
	//   - Request cancellation if the client disconnects
	//   - Timeout handling for long-running requests
	//   - Passing request-scoped metadata (tracing, authentication, etc.)
	Call(ctx context.Context, req Request) (Response, error)
}

// StreamingModel provides a generic API for invoking AI models with streaming
// responses. It abstracts the process of sending requests and receiving responses
// incrementally, chunk by chunk. The interface uses Go generics to accommodate
// different request and response chunk types, enhancing flexibility and
// adaptability across different AI model implementations.
//
// StreamingModel is particularly useful for:
//   - Real-time chat applications where responses appear incrementally
//   - Long-form content generation where users want to see progress
//   - Large batch processing where memory efficiency is important
//   - Interactive applications requiring immediate feedback
//
// The streaming approach provides several benefits:
//   - Improved user experience with real-time feedback
//   - Memory efficiency by processing responses incrementally
//   - Better resource utilization through backpressure handling
//   - Early termination capability when response is satisfactory
type StreamingModel[Request any, Response any] interface {
	// Stream executes a request to the AI model and returns an iterator for
	// receiving response chunks incrementally. This allows real-time processing
	// of the model's output as it becomes available.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeouts, and request-scoped values
	//   - req: The request object to send to the AI model
	//
	// Returns:
	//   - iter.Seq2[Response, error]: An iterator yielding response chunks and errors
	//
	// The returned iterator allows you to:
	//   - Process response chunks as they become available
	//   - Handle errors gracefully during streaming
	//   - Terminate early by breaking from the iteration
	//   - Respect context cancellation and timeouts
	//
	// Usage example:
	//   for chunk, err := range model.Stream(ctx, request) {
	//       if err != nil {
	//           return fmt.Errorf("streaming error: %w", err)
	//       }
	//       // Process chunk
	//       fmt.Print(chunk)
	//   }
	Stream(ctx context.Context, req Request) iter.Seq2[Response, error]
}
