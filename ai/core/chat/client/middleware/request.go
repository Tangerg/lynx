package middleware

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

type ChatRequestMode string

const (
	CallRequest   ChatRequestMode = "call"
	StreamRequest ChatRequestMode = "stream"
)

// Request is a generic struct representing a chat request in a chat application.
// It is parameterized by chat options (O) and chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Fields:
//   - ChatModel: An instance of model.ChatModel[O, M], representing the chat model used for processing the request.
//   - ChatRequestOptions: An instance of type O, containing options specific to the chat session.
//   - UserText: A string containing the text input from the user.
//   - UserParams: A map for storing arbitrary key-value pairs related to user-specific parameters.
//   - SystemText: A string containing system-generated text or instructions.
//   - SystemParams: A map for storing arbitrary key-value pairs related to system-specific parameters.
//   - Messages: A slice of message.ChatMessage, representing the sequence of messages in the chat session.
//   - Mode: An instance of ChatRequestMode, indicating the mode of the chat request (e.g., call or stream).
type Request[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	ChatModel          model.ChatModel[O, M]
	ChatRequestOptions O
	UserText           string
	UserParams         map[string]any
	UserMedia          []*media.Media
	SystemText         string
	SystemParams       map[string]any
	Messages           []message.ChatMessage
	Mode               ChatRequestMode
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
