package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/core/model"
)

// Model is a generic interface representing a model capable of processing prompts
// and generating completions. It is parameterized by two types:
//
// Type Parameters:
//   - O: Represents the options for the prompt. This type is expected to conform to
//     request.ChatRequestOptions.
//   - M: Represents the metadata for the generation of the completion. This type is
//     expected to conform to result.ChatResultMetadata.
//
// The interface embeds the model.Model interface, which specifies the core behavior
// of a model. The embedded interface is parameterized with:
//
// - *request.ChatRequest[O]: A pointer to a ChatRequest that uses the options type O.
// - *response.ChatResponse[M]: A pointer to a ChatResponse that uses the metadata type M.
//
// This design allows the `Model` interface to provide additional capabilities while
// inheriting the core functionality defined by model.Model.
//
// Example Implementation:
//
//	type ChatModel struct {}
//
//	func (m *ChatModel) Call(
//	    ctx context.Context,
//	    req *request.ChatRequest[ChatOptions],
//	) (*response.ChatResponse[ChatMetadata], error) {
//	    // Implementation for processing the request and generating a response.
//	}
//
// Example Usage:
//
//	var model Model[ChatOptions, ChatMetadata]
//	response, err := model.Call(ctx, chatRequest)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(response)
type Model[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	model.Model[*request.ChatRequest[O], *response.ChatResponse[M]]
}

// StreamingModel is a generic interface that extends the model.StreamingModel interface,
// representing a streaming model capable of processing prompts and generating completions
// in a streaming manner. It is parameterized by two types:
//
// Type Parameters:
//   - O: Represents the options used in the prompt. This is typically a struct or type
//     containing configuration or settings that define how the prompt is generated or processed.
//   - M: Represents the metadata associated with the generation process. This is typically
//     a struct or type that provides additional context or information about the completion process.
//
// Underlying Interface:
//
//	StreamingModel embeds the model.StreamingModel interface, which operates on:
//	- *request.ChatRequest[O]: A pointer to a ChatRequest that uses the options type O.
//	- *response.ChatResponse[M]: A pointer to a ChatResponse that uses the metadata type M.
//
// This design enables the StreamingModel interface to inherit the core streaming behavior
// while allowing for the use of customizable options and metadata types.
//
// Usage:
// StreamingModel is typically implemented by types that perform streaming operations
// where prompts are processed to produce responses incrementally. This is particularly useful
// in scenarios where results need to be delivered in real time, such as conversational AI
// or large text generation.
//
// Example Implementation:
//
//	type ChatStreamingModel struct {}
//
//	func (m *ChatStreamingModel) Stream(
//	    ctx context.Context,
//	    req *request.ChatRequest[ChatOptions],
//	    handler func(*response.ChatResponse[ChatMetadata]) error,
//	) (*response.ChatResponse[ChatMetadata], error) {
//	    // Implementation for streaming the response in real time.
//	}
//
// Example Usage:
//
//	var streamingModel StreamingModel[ChatOptions, ChatMetadata]
//	err := streamingModel.Stream(ctx, chatRequest, func(resp *response.ChatResponse[ChatMetadata]) error {
//	    fmt.Println("Received chunk:", resp)
//	    return nil
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
type StreamingModel[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	model.StreamingModel[*request.ChatRequest[O], *response.ChatResponse[M]]
}
