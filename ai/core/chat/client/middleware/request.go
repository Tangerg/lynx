package middleware

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type RequestMode string

const (
	CallRequest   RequestMode = "call"
	StreamRequest RequestMode = "stream"
)

type Request[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	ChatModel    model.ChatModel[O, M]
	ChatOptions  O
	UserText     string
	UserParams   map[string]any
	SystemText   string
	SystemParams map[string]any
	Messages     []message.ChatMessage
	Mode         RequestMode
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
func (r *Request[O, M]) AddUserMessage(text string) {
	msg := message.NewUserMessage(text)
	r.AddMessage(msg)
}
func (r *Request[O, M]) AddSystemMessage(text string) {
	msg := message.NewSystemMessage(text)
	r.AddMessage(msg)
}
