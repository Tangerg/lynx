package messages

import (
	"github.com/Tangerg/lynx/ai/commons/content"
)

var (
	_ Message              = (*UserMessage)(nil)
	_ content.MediaContent = (*UserMessage)(nil)
)

// UserMessage represents a message from the end-user or developer.
// They represent questions, prompts, or any input that you want
// the AI to respond to. UserMessage can contain both text content
// and media attachments such as images, audio, or documents.
type UserMessage struct {
	message
	media []*content.Media // Media attachments for the message
}

// HasMedia returns true if the message contains any media attachments.
func (u *UserMessage) HasMedia() bool {
	return len(u.media) > 0
}

// Media returns the media attachments of the message.
func (u *UserMessage) Media() []*content.Media {
	return u.media
}

type UserMessageParam struct {
	Text     string
	Media    []*content.Media
	Metadata map[string]any
}

// NewUserMessage creates a new user message using Go generics to simulate function overloading.
// This allows creating user messages with different parameter types in a single function call.
//
// Supported parameter types:
//   - string: Sets the text content
//   - []*content.Media: Sets media attachments
//   - UserMessageParam: Complete parameter struct with text, media, and metadata fields
//
// The function uses type constraints and type switching to handle different input types,
// providing a convenient API that mimics function overloading found in other languages.
//
// Examples:
//
//	NewUserMessage("Hello, how are you?")               // Text only
//	NewUserMessage(mediaSlice)                          // Media only
//	NewUserMessage(UserMessageParam{...})               // Full configuration
func NewUserMessage[T string | []*content.Media | UserMessageParam](param T) *UserMessage {
	var p UserMessageParam

	switch typedParam := any(param).(type) {
	case string:
		p.Text = typedParam
	case []*content.Media:
		p.Media = typedParam
	case UserMessageParam:
		p = typedParam
	}

	if p.Media == nil {
		p.Media = make([]*content.Media, 0)
	}

	return &UserMessage{
		message: newMessage(User, p.Text, p.Metadata),
		media:   p.Media,
	}
}
