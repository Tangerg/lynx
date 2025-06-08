package model

import (
	"github.com/Tangerg/lynx/ai/model/chat/request"
	"github.com/Tangerg/lynx/ai/model/chat/response"
	"github.com/Tangerg/lynx/ai/model/model"
)

// ChatModel provides a specialized interface for chat-based AI models that combines
// both synchronous and streaming capabilities. It extends the generic Model interfaces
// by specifying concrete types for chat interactions, making it easier to work with
// conversational AI models while maintaining type safety and flexibility.
//
// This interface is designed to work with various chat model implementations such as:
//   - OpenAI GPT models (GPT-3.5, GPT-4, etc.)
//   - Anthropic Claude models
//   - Google Gemini/Bard models
//   - Local chat models (Llama, Mistral, etc.)
//   - Custom chat model implementations
//
// The interface provides two interaction modes:
//
// 1. Synchronous Mode (via embedded Model interface):
//   - Suitable for simple Q&A scenarios
//   - Batch processing of chat requests
//   - Testing and development
//   - Applications where complete response is needed before proceeding
//
// 2. Streaming Mode (via embedded StreamingModel interface):
//   - Real-time chat applications
//   - Interactive conversational interfaces
//   - Long-form content generation with live feedback
//   - Memory-efficient processing of large responses
//
// Type Parameters:
//   - Request: [*request.ChatRequest] - Encapsulates chat messages, system prompts,
//     temperature, max tokens, and other chat-specific configuration options
//   - Response: [*response.ChatResponse] - Contains the model's response message, usage statistics,
//     finish reason, and other metadata
//
// Usage Examples:
//
//	// Synchronous chat
//	response, err := chatModel.Call(ctx, chatRequest)
//	if err != nil {
//	    return err
//	}
//	// Process response
//
//	// Streaming chat
//	reader, err := chatModel.Stream(ctx, chatRequest)
//	if err != nil {
//	    return err
//	}
//
//	for {
//	    chunk, err := reader.Read(ctx)
//	    if err == io.EOF {
//	        break
//	    }
//	    if err != nil {
//	        return err
//	    }
//	    // Process chunk
//	}
//
// The interface leverages Go's type system to ensure compile-time safety while
// providing maximum flexibility for different chat model implementations. This
// design allows developers to easily switch between different chat model providers
// or implementations without changing the application code.
type ChatModel interface {
	model.Model[*request.ChatRequest, *response.ChatResponse]
	model.StreamingModel[*request.ChatRequest, *response.ChatResponse]
	// DefaultOptions returns the default configuration options for this chat model.
	// These options include model-specific defaults such as temperature, max tokens,
	// top-p sampling, presence penalty, frequency penalty, and other parameters
	// that are optimized for the particular model implementation.
	//
	// The returned options can be used as a baseline configuration and modified
	// as needed for specific requests. This provides a convenient way to get
	// reasonable defaults without having to specify all parameters manually.
	DefaultOptions() request.ChatOptions
}
