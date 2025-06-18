package model

import (
	"context"

	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/stream"
)

// Model provides a generic API for invoking AI models. It is designed to
// handle the interaction with various types of AI models by abstracting the
// process of sending requests and receiving responses. The interface uses Go
// generics to accommodate different types of requests and responses, enhancing
// flexibility and adaptability across different AI model implementations.
//
// The Model interface follows a simple request-response pattern where each
// call is synchronous and returns a complete response. This is suitable for
// scenarios where you need the full result before proceeding, such as:
//   - Single-turn conversations or Q&A
//   - Batch processing of embeddings
//   - Image generation requests
//   - Classification or analysis tasks
//
// For streaming responses, use StreamingModel instead.
type Model[Request any, Response any] interface {
	// Call executes a method call to the AI model and returns the complete response.
	// The method is synchronous and will block until the model has generated the
	// full response or an error occurs.
	//
	// Parameters:
	//   ctx: Context for cancellation, timeouts, and request-scoped values
	//   req: The request object to be sent to the AI model. must implement the Request interface.
	//
	// Returns:
	//   Response: The complete response from the AI model. must implement the Response interface.
	//   error: Any error that occurred during the model invocation
	//
	// The context allows for:
	//   - Request cancellation if the client disconnects
	//   - Timeout handling for long-running requests
	//   - Passing request-scoped metadata (tracing, authentication, etc.)
	Call(ctx context.Context, req Request) (Response, error)
}

// StreamingModel provides a generic API for invoking AI models with streaming
// responses. It abstracts the process of sending requests and receiving streaming
// responses chunk by chunk. The interface uses Go generics to accommodate different
// types of requests and response chunks, enhancing flexibility and adaptability
// across different AI model implementations.
//
// Streaming models are particularly useful for:
//   - Real-time chat applications where responses appear incrementally
//   - Long-form content generation where users want to see progress
//   - Large batch processing where memory efficiency is important
//   - Interactive applications requiring immediate feedback
//
// The streaming approach provides several benefits:
//   - Improved user experience with real-time feedback
//   - Memory efficiency by processing responses incrementally
//   - Better resource utilization through backpressure handling
//   - Early termination capability if the response is satisfactory
type StreamingModel[Request any, Response any] interface {
	// Stream executes a method call to the AI model and returns a stream reader
	// for receiving response chunks incrementally. This allows for real-time
	// processing of the model's output as it becomes available.
	//
	// Parameters:
	//   ctx: Context for cancellation, timeouts, and request-scoped values
	//   req: The request object to be sent to the AI model. must implement the Request interface.
	//
	// Returns:
	//
	// Returns:
	//   stream.Reader[Response]: A stream reader for consuming response chunks
	//   - Response: The complete response from the AI model. must implement the Response interface.
	//   error: Any error that occurred during the initial request setup
	//
	// The returned stream.Reader allows you to:
	//   - Read response chunks as they become available
	//   - Handle backpressure by controlling read speed
	//   - Close the stream early if needed
	//   - Handle streaming errors gracefully
	//
	// Usage example:
	//   reader, err := model.Stream(ctx, request)
	//   if err != nil {
	//       return err
	//   }
	//
	//   for {
	//       chunk, err := reader.Read(ctx)
	//       if err == io.EOF {
	//           break
	//       }
	//       if err != nil {
	//           return err
	//       }
	//       // Process chunk
	//   }
	Stream(ctx context.Context, req Request) (stream.Reader[result.Result[Response]], error)
}
