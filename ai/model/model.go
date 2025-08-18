package model

// Model provides a generic API for invoking AI models with synchronous
// request-response patterns. It extends CallHandler to provide a complete
// AI model abstraction that handles the process of sending requests and
// receiving complete responses. The interface uses Go generics to accommodate
// different request and response types, enhancing flexibility and adaptability
// across different AI model implementations.
//
// By embedding CallHandler, Model inherits the core functionality for
// synchronous AI model interactions while maintaining a clear semantic
// distinction as a complete AI model interface. This design allows Model
// to be used interchangeably with CallHandler while providing additional
// model-specific context and behavior.
//
// The Model interface follows a simple request-response pattern where each
// call is synchronous and returns a complete response. This is suitable for
// scenarios where you need the full result before proceeding, such as:
//   - Single-turn conversations or Q&A sessions
//   - Batch processing of embeddings
//   - Image generation requests
//   - Classification or analysis tasks
//   - Function calling with complete responses
//   - Model evaluation and benchmarking
//
// For streaming responses where you need incremental output, use StreamingModel instead.
type Model[Request any, Response any] interface {
	// CallHandler provides the core synchronous call functionality.
	// Enables direct usage wherever CallHandler is expected and seamless
	// integration with middleware chains designed for CallHandler.
	CallHandler[Request, Response]
}

// StreamingModel provides a generic API for invoking AI models with streaming
// responses. It extends StreamHandler to provide a complete AI model abstraction
// that handles the process of sending requests and receiving responses incrementally,
// chunk by chunk. The interface uses Go generics to accommodate different request
// and response chunk types, enhancing flexibility and adaptability across different
// AI model implementations.
//
// By embedding StreamHandler, StreamingModel inherits the core streaming
// functionality while maintaining a clear semantic distinction as a complete
// streaming AI model interface. This design allows StreamingModel to be used
// interchangeably with StreamHandler while providing additional model-specific
// context and behavior.
//
// StreamingModel is particularly useful for:
//   - Real-time chat applications where responses appear incrementally
//   - Long-form content generation where users want to see progress
//   - Large batch processing where memory efficiency is important
//   - Interactive applications requiring immediate feedback
//   - Server-sent events for AI model responses
//   - Live transcription or translation services
//
// The streaming approach provides several benefits:
//   - Improved user experience with real-time feedback
//   - Memory efficiency by processing responses incrementally
//   - Better resource utilization through backpressure handling
//   - Early termination capability when response is satisfactory
//   - Reduced time-to-first-byte for better perceived performance
type StreamingModel[Request any, Response any] interface {
	// StreamHandler provides the core streaming functionality.
	// Enables direct usage wherever StreamHandler is expected and seamless
	// integration with streaming middleware chains for model implementations.
	StreamHandler[Request, Response]
}
