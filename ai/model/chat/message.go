package chat

import (
	"github.com/Tangerg/lynx/ai/content"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

type MessageType = messages.Type

const (
	MessageTypeSystem    = messages.System
	MessageTypeAssistant = messages.Assistant
	MessageTypeUser      = messages.User
	MessageTypeTool      = messages.Tool
)

type Message = messages.Message

func NewMessage(params MessageParams) (Message, error) {
	return messages.NewMessage(params)
}

type MessageParams = messages.MessageParams

type ToolCall = messages.ToolCall

type ToolReturn = messages.ToolReturn

type SystemMessage = messages.SystemMessage

func NewSystemMessage[T string | MessageParams](param T) *SystemMessage {
	return messages.NewSystemMessage(param)
}

type AssistantMessage = messages.AssistantMessage

func NewAssistantMessage[T string | []*content.Media | []*ToolCall | map[string]any | MessageParams](param T) *AssistantMessage {
	return messages.NewAssistantMessage(param)
}

type UserMessage = messages.UserMessage

func NewUserMessage[T string | []*content.Media | MessageParams](param T) *UserMessage {
	return messages.NewUserMessage(param)
}

type ToolMessage = messages.ToolMessage

func NewToolMessage[T []*ToolReturn | MessageParams](param T) (*ToolMessage, error) {
	return messages.NewToolMessage(param)
}
