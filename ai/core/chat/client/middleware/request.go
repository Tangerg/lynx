package middleware

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	baseModel "github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

// Request is a generic struct representing a chat request in a chat application.
// It encapsulates all the necessary information for processing a chat interaction,
// including user inputs, system instructions, chat options, and metadata.
//
// Type Parameters:
//   - O: Represents the chat options, defined by request.ChatRequestOptions.
//   - M: Represents the metadata associated with chat generation, defined by result.ChatResultMetadata.
//
// Fields:
//   - ChatModel: An instance of model.ChatModel[O, M]. This represents the chat model used for
//     processing the request and generating responses.
//   - ChatRequestOptions: An instance of type O, containing options and configurations specific
//     to the chat session.
//   - UserText: A string containing the user-provided text input for the chat interaction.
//   - UserParams: A map of arbitrary key-value pairs associated with user-specific parameters
//     that can influence the request processing.
//   - UserMedia: A slice of pointers to media.Media, representing media inputs (e.g., images,
//     audio) provided by the user.
//   - SystemText: A string containing system-generated text or instructions, such as guidelines
//     or predefined prompts for the chat model.
//   - SystemParams: A map of arbitrary key-value pairs associated with system-specific parameters
//     that can influence the chat behavior or response generation.
//   - Messages: A slice of message.ChatMessage, representing the sequence of messages exchanged
//     in the chat session, including both user and system messages.
//   - Mode: An instance of ChatRequestMode, indicating the mode of the chat request. Possible
//     values include `CallRequest` for synchronous processing and `StreamRequest` for
//     real-time streaming interactions.
//   - StreamChunkHandler: A baseModel.StreamChunkHandler that processes streaming chunks of the
//     chat response. It is used in streaming mode to handle incremental
//     responses as they are generated.
//
// Usage:
// This struct is used to encapsulate all input and contextual information required to process
// a chat request, enabling flexible configurations for both synchronous and streaming modes.
//
// Example Usage:
//
//	req := &Request[ChatOptions, ChatMetadata]{
//	    ChatModel: myChatModel,
//	    ChatRequestOptions: ChatOptions{
//	        Temperature: 0.7,
//	    },
//	    UserText: "Hello, how can I help?",
//	    UserParams: map[string]any{
//	        "user_id": "12345",
//	    },
//	    SystemText: "This is an AI chat assistant.",
//	    SystemParams: map[string]any{
//	        "assistant_version": "1.0",
//	    },
//	    Messages: []message.ChatMessage{
//	        {Role: "user", Content: "Hi!"},
//	    },
//	    Mode: CallRequest,
//	}
type Request[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	// ChatModel represents the chat model used for processing the request.
	ChatModel model.ChatModel[O, M]

	// ChatRequestOptions contains options specific to the chat session.
	ChatRequestOptions O

	// UserText contains the text input from the user.
	UserText string

	// UserParams stores arbitrary key-value pairs related to user-specific parameters.
	UserParams map[string]any

	// UserMedia represents media inputs (e.g., images, audio) provided by the user.
	UserMedia []*media.Media

	// SystemText contains system-generated text or instructions.
	SystemText string

	// SystemParams stores arbitrary key-value pairs related to system-specific parameters.
	SystemParams map[string]any

	// Messages represents the sequence of messages in the chat session.
	Messages []message.ChatMessage

	// Mode indicates the mode of the chat request (e.g., call or stream).
	Mode ChatRequestMode

	// StreamChunkHandler processes streaming chunks of the chat response.
	StreamChunkHandler baseModel.StreamChunkHandler[*response.ChatResponse[M]]
}

func (r *Request[O, M]) IsCall() bool {
	return r.Mode == CallRequest
}

func (r *Request[O, M]) IsStream() bool {
	return r.Mode == StreamRequest
}

func (r *Request[O, M]) UserParam(key string) (any, bool) {
	val, ok := r.UserParams[key]
	return val, ok
}

func (r *Request[O, M]) SetUserParam(key string, val any) {
	r.UserParams[key] = val
}

func (r *Request[O, M]) SystemParam(key string) (any, bool) {
	val, ok := r.SystemParams[key]
	return val, ok
}

func (r *Request[O, M]) SetSystemParam(key string, val any) {
	r.SystemParams[key] = val
}

func (r *Request[O, M]) AddMessage(msg ...message.ChatMessage) {
	r.Messages = append(r.Messages, msg...)
}

func (r *Request[O, M]) AddUserMessage(text string, metadata map[string]any, m ...*media.Media) {
	msg := message.NewUserMessage(text, metadata, m...)
	r.AddMessage(msg)
}

func (r *Request[O, M]) AddSystemMessage(text string, metadata map[string]any) {
	msg := message.NewSystemMessage(text, metadata)
	r.AddMessage(msg)
}
