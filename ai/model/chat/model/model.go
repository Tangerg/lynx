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
//   - Request: ChatRequest[ChatOptions] - Encapsulates chat messages, system prompts,
//     temperature, max tokens, and other chat-specific configuration options
//   - Response: ChatResponse - Contains the model's response message, usage statistics,
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
	model.Model[request.ChatRequest[request.ChatOptions], response.ChatResponse]
	model.StreamingModel[request.ChatRequest[request.ChatOptions], response.ChatResponse]
}
