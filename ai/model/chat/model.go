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
// Request/Response Structure:
//   - Request: Chat request containing messages, system prompts, and LLM parameters
//   - Response: Contains one or more Results with assistant messages, metadata, and optional tool execution results
//   - Result: Single generation result with AssistantMessage, ResultMetadata, and optional ToolMessage
//   - ResponseMetadata: Usage statistics (token consumption), rate limits, and provider-specific metadata
//
// Finish Reasons:
// The ResultMetadata provides finish reasons to understand why generation stopped:
//   - FinishReasonStop: Natural completion or stop sequence reached
//   - FinishReasonLength: Truncated due to token limit
//   - FinishReasonToolCalls: Stopped to execute tool/function calls
//   - FinishReasonContentFilter: Response blocked by safety filters
//   - FinishReasonReturnDirect: Direct tool result return without further generation
//
// Streaming Benefits:
//   - Improved user experience with real-time feedback
//   - Memory efficiency by processing responses incrementally
//   - Better resource utilization through backpressure handling
//   - Early termination capability when response is satisfactory
//   - Context cancellation and timeout support
//
// The interface abstracts LLM provider differences while maintaining access to
// provider-specific features through the Options system and metadata Extra fields.
// Both ResultMetadata and ResponseMetadata support custom key-value pairs for
// provider-specific information through their Get/Set methods.
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

	// Info returns basic information about the model, including the provider name.
	// This can be used to identify which LLM provider is being used and implement
	// provider-specific logic or logging when needed.
	Info() ModelInfo
}

// ModelInfo contains basic metadata about the LLM model instance.
type ModelInfo struct {
	// Provider identifies the LLM service provider (e.g., "OpenAI", "Anthropic", "Google").
	// Used for provider-specific feature detection and analytics.
	Provider string `json:"provider"`
}
