package model

import (
	"context"
	"iter"
)

// CallHandler provides a generic API for invoking AI models with synchronous
// request-response patterns. It abstracts the interaction with various types
// of AI models by handling the process of sending requests and receiving
// complete responses. The interface uses Go generics to accommodate different
// request and response types, enhancing flexibility and adaptability across
// different AI model implementations.
//
// CallHandler is suitable for scenarios where you need the full result before
// proceeding, such as:
//   - Single-turn conversations or Q&A sessions
//   - Batch processing of embeddings
//   - Image generation requests
//   - Classification or analysis tasks
//   - Function calling with complete responses
type CallHandler[Request any, Response any] interface {
	// Call executes a request to the AI model and returns the complete response.
	// This method is synchronous and blocks until the model generates the full
	// response or an error occurs.
	Call(ctx context.Context, req Request) (Response, error)
}

// CallHandlerFunc is a function type that implements the CallHandler interface.
// It allows regular functions to be used as CallHandler implementations,
// providing a convenient way to create handlers without defining new types.
type CallHandlerFunc[Request any, Response any] func(ctx context.Context, req Request) (Response, error)

// Call implements the CallHandler interface for CallHandlerFunc.
// It delegates to the underlying function, allowing function types
// to be used wherever a CallHandler is expected.
func (c CallHandlerFunc[Request, Response]) Call(ctx context.Context, req Request) (Response, error) {
	return c(ctx, req)
}

// StreamHandler provides a generic API for invoking AI models with streaming
// responses. It abstracts the process of sending requests and receiving responses
// incrementally, chunk by chunk. The interface uses Go generics to accommodate
// different request and response chunk types, enhancing flexibility and
// adaptability across different AI model implementations.
//
// StreamHandler is particularly useful for:
//   - Real-time chat applications where responses appear incrementally
//   - Long-form content generation where users want to see progress
//   - Large batch processing where memory efficiency is important
//   - Interactive applications requiring immediate feedback
//   - Server-sent events for AI model responses
//
// The streaming approach provides several benefits:
//   - Improved user experience with real-time feedback
//   - Memory efficiency by processing responses incrementally
//   - Better resource utilization through backpressure handling
//   - Early termination capability when response is satisfactory
type StreamHandler[Request any, Response any] interface {
	// Stream executes a request to the AI model and returns an iterator for
	// receiving response chunks incrementally. This allows real-time processing
	// of the model's output as it becomes available.
	Stream(ctx context.Context, req Request) iter.Seq2[Response, error]
}

// StreamHandlerFunc is a function type that implements the StreamHandler interface.
// It allows regular functions to be used as StreamHandler implementations,
// providing a convenient way to create streaming handlers without defining new types.
type StreamHandlerFunc[Request any, Response any] func(ctx context.Context, req Request) iter.Seq2[Response, error]

// Stream implements the StreamHandler interface for StreamHandlerFunc.
// It delegates to the underlying function, allowing function types
// to be used wherever a StreamHandler is expected.
func (s StreamHandlerFunc[Request, Response]) Stream(ctx context.Context, req Request) iter.Seq2[Response, error] {
	return s(ctx, req)
}
