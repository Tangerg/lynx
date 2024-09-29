package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/model"
)

// StreamingModel is an interface that represents a streaming model for processing messages.
// It is parameterized by O, which represents the options for the prompt, and M,
// which represents the metadata for generation.
//
// This interface extends the model.StreamingModel interface, which takes the following type parameters:
//   - []message.Message: A slice of Message objects that the model will process in a streaming manner.
//   - O: The options type that provides configuration for the prompt.
//   - *message.AssistantMessage: A pointer to an AssistantMessage, which is the type of message
//     that the model will produce as output in a streaming fashion.
//   - M: The type of metadata that will be associated with the generation process.
//
// The StreamingModel interface is designed for scenarios where messages are processed and generated
// in a continuous stream, allowing for real-time interaction and feedback.
type StreamingModel[O prompt.Options, M metadata.GenerationMetadata] interface {
	model.StreamingModel[[]message.Message, O, *message.AssistantMessage, M]
}
