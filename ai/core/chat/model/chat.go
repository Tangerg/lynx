package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// ChatModel is a generic interface representing a chat model capable of handling both
// batch and streaming message processing. It is parameterized by two types:
//
// Type Parameters:
//   - O: Represents the options for the prompt, typically containing configuration
//     or settings for generating chat messages.
//   - M: Represents the metadata for the generation process, providing additional
//     context or details about the output.
//
// Functionalities:
// ChatModel combines the capabilities of both the `Model` and `StreamingModel` interfaces:
//
//   - Model[O, M]:
//     Supports batch message processing, where messages are processed and generated as a
//     complete set in response to a given input.
//
//   - StreamingModel[O, M]:
//     Enables streaming message processing, where messages are processed and generated
//     incrementally, supporting real-time interactions.
//
// Applications:
// This interface is designed for scenarios requiring flexible message processing, such as:
//   - Batch processing of chat messages for applications like customer service reports or
//     document summarization.
//   - Streaming interactions for real-time conversational AI or chatbots.
//
// Example Implementation:
//
//	type MyChatModel struct {}
//
//	func (m *MyChatModel) Call(
//	    ctx context.Context,
//	    req *request.ChatRequest[ChatOptions],
//	) (*response.ChatResponse[ChatMetadata], error) {
//	    // Implementation for batch message processing.
//	}
//
//	func (m *MyChatModel) Stream(
//	    ctx context.Context,
//	    req *request.ChatRequest[ChatOptions],
//	    handler func(*response.ChatResponse[ChatMetadata]) error,
//	) (*response.ChatResponse[ChatMetadata], error) {
//	    // Implementation for streaming message processing.
//	}
//
// Example Usage:
//
//	var chatModel ChatModel[ChatOptions, ChatMetadata]
//
//	// Batch processing
//	response, err := chatModel.Call(ctx, chatRequest)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Batch response:", response)
//
//	// Streaming processing
//	err = chatModel.Stream(ctx, chatRequest, func(resp *response.ChatResponse[ChatMetadata]) error {
//	    fmt.Println("Streaming response chunk:", resp)
//	    return nil
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
type ChatModel[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	Model[O, M]
	StreamingModel[O, M]
}
