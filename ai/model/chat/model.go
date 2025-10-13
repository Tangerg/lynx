package chat

import (
	"github.com/Tangerg/lynx/ai/model"
)

// Model defines the interface for LLM chat models supporting both synchronous and streaming interactions.
// Provides type-safe access to conversational AI capabilities with comprehensive configuration options.
//
// Supports major LLM providers and deployment options:
//   - Cloud LLMs: OpenAI GPT, Anthropic Claude, Google Gemini
//   - Open source LLMs: Llama, Mistral, CodeLlama
//   - Local deployments: Ollama, vLLM, custom endpoints
//   - Enterprise solutions: Azure OpenAI, AWS Bedrock
//
// Interaction Modes:
//
// 1. Synchronous Mode:
//   - Complete response generation before returning
//   - Ideal for batch processing and simple Q&A
//   - Better for applications requiring full context before proceeding
//   - Easier error handling and response validation
//
// 2. Streaming Mode:
//   - Token-by-token response streaming using Go iterators
//   - Real-time chat interfaces and live content generation
//   - Reduced perceived latency for long responses
//   - Memory efficient for large LLM outputs
//   - Built-in backpressure handling and early termination support
//
// MessageType Parameters:
//   - Request: Chat request containing messages, system prompts, and LLM parameters
//   - Response: LLM response with generated content, usage statistics, and metadata
//
// Usage Examples:
//
//	// Synchronous LLM interaction
//	response, err := llmModel.Call(ctx, request)
//	if err != nil {
//	    return err
//	}
//	content := response.Result().Output().Text()
//
//	// Streaming LLM interaction
//	for chunk, err := range llmModel.Stream(ctx, request) {
//	    if err != nil {
//	        return fmt.Errorf("streaming error: %w", err)
//	    }
//	    // Process streaming chunk
//	    fmt.Print(chunk.Result().Output().Text())
//	}
//
// The streaming approach provides several benefits:
//   - Improved user experience with real-time feedback
//   - Memory efficiency by processing responses incrementally
//   - Better resource utilization through backpressure handling
//   - Early termination capability when response is satisfactory
//   - ToolContext cancellation and timeout support
//
// The interface abstracts LLM provider differences while maintaining access to
// provider-specific features through the Options and metadata systems.
type Model interface {
	model.Model[*Request, *Response]
	model.StreamingModel[*Request, *Response]

	// DefaultOptions returns optimized default parameters for this LLM.
	// Includes model-specific settings for temperature, token limits, sampling parameters,
	// and penalties that are tuned for optimal performance with the specific LLM.
	//
	// These defaults provide a good starting point and can be customized per request.
	// Useful for maintaining consistent behavior across different LLM interactions.
	DefaultOptions() *Options
}
