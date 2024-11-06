package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// ChatModel is an interface representing a chat model capable of both batch and streaming message processing.
// It is parameterized by O, which represents the options for the prompt, and M, which represents the metadata for generation.
//
// This interface combines the functionalities of both Model and StreamingModel interfaces,
// allowing it to handle message processing in both batch and streaming modes.
//
//   - Model[O, M]: Provides the capability to process messages in a batch mode, where messages are
//     processed and generated as a complete set.
//   - StreamingModel[O, M]: Provides the capability to process messages in a streaming mode, where
//     messages are processed and generated continuously, allowing for real-time interaction.
//
// The ChatModel interface is designed for applications requiring flexible message processing
// capabilities, supporting both traditional batch processing and modern streaming interactions.
type ChatModel[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	Model[O, M]
	StreamingModel[O, M]
}
