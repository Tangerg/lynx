package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/model"
)

// StreamingModel is a generic interface that extends the model.StreamingModel interface.
// It is parameterized with two types, O and M, which represent options and metadata, respectively.
//
// Type Parameters:
//   - O: Represents the type of options used in the prompt. This is typically a struct or type
//     that holds configuration or settings for generating the prompt.
//   - M: Represents the type of metadata associated with the generation process. This is typically
//     a struct or type that holds additional information about the generation process.
//
// This interface is designed to work with a specific streaming model that takes a prompt and
// returns a completion, both of which are parameterized with the types O and M.
//
// Underlying Interface:
//   - model.StreamingModel[*prompt.ChatPrompt[O], *completion.ChatCompletion[M]]:
//     This indicates that the StreamingModel interface is based on another interface that
//     operates with pointers to prompt.ChatPrompt and completion.ChatCompletion, parameterized with
//     the types O and M, respectively.
//
// Usage:
//
//	This interface is typically implemented by types that need to perform streaming operations
//	where a prompt is processed to generate a completion, with both the prompt and completion
//	being customizable through the use of options and metadata.
type StreamingModel[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	model.StreamingModel[*prompt.ChatPrompt[O], *completion.ChatCompletion[M]]
}
