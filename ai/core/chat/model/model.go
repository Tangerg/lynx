package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/model"
)

// Model is an interface that represents a generic model for processing messages.
// It is parameterized by O, which represents the options for the prompt, and M,
// which represents the metadata for generation.
//
// This interface extends the model.Model interface, which takes the following type parameters:
//   - []message.Message: A slice of Message objects that the model will process.
//   - O: The options type that provides configuration for the prompt.
//   - *message.AssistantMessage: A pointer to an AssistantMessage, which is the type of message
//     that the model will produce as output.
//   - M: The type of metadata that will be associated with the generation process.
type Model[O prompt.Options, M metadata.GenerationMetadata] interface {
	model.Model[[]message.Message, O, *message.AssistantMessage, M]
}
