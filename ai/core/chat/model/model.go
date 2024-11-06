package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/core/model"
)

// Model is a generic interface representing a model capable of processing prompts
// and generating completions. It is parameterized by two types:
// O - Represents the options for the prompt.
// M - Represents the metadata for the generation of the completion.
type Model[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	// The Model interface embeds another interface, model.Model, which is
	// parameterized with specific types:
	// *prompt.ChatPrompt[O] - A pointer to a ChatPrompt struct that uses the options type O.
	// *completion.ChatCompletion[M] - A pointer to a ChatCompletion struct that uses the metadata type M.
	model.Model[*request.ChatRequest[O], *response.ChatResponse[M]]
}

// StreamingModel is a generic interface extending the model.StreamingModel interface.
// It is parameterized with two types, O and M, representing options and metadata, respectively.
//
// Type Parameters:
//   - O: Represents the type of options used in the prompt, typically a struct or type
//     holding configuration or settings for generating the prompt.
//   - M: Represents the type of metadata associated with the generation process, typically
//     a struct or type holding additional information about the generation process.
//
// This interface is designed to work with a specific streaming model that takes a prompt and
// returns a completion, both parameterized with the types O and M.
//
// Underlying Interface:
//   - model.StreamingModel[*prompt.ChatPrompt[O], *completion.ChatCompletion[M]]:
//     Indicates that the StreamingModel interface is based on another interface operating with
//     pointers to prompt.ChatPrompt and completion.ChatCompletion, parameterized with
//     the types O and M, respectively.
//
// Usage:
//
//	This interface is typically implemented by types performing streaming operations
//	where a prompt is processed to generate a completion, with both the prompt and completion
//	being customizable through the use of options and metadata.
type StreamingModel[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	model.StreamingModel[*request.ChatRequest[O], *response.ChatResponse[M]]
}
