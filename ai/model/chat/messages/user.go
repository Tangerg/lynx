package messages

import (
	"slices"

	"github.com/Tangerg/lynx/ai/content"
)

var (
	_ Message              = (*UserMessage)(nil)
	_ content.MediaContent = (*UserMessage)(nil)
)

// UserMessage represents a message from an end-user or developer in the conversation.
// User messages typically contain questions, prompts, requests, or any input that
// requires a response from the AI assistant. UserMessage supports both text content
// and media attachments such as images, audio files, documents, or other multimedia content.
type UserMessage struct {
	message
	media []*content.Media // Media attachments included with the user's message
}

// Type returns the message type as User.
func (u *UserMessage) Type() Type {
	return User
}

// HasMedia reports whether the message contains any media attachments.
func (u *UserMessage) HasMedia() bool {
	return len(u.media) > 0
}

// Media returns a slice of media attachments in the message.
func (u *UserMessage) Media() []*content.Media {
	return u.media
}

// NewUserMessage creates a new user message using Go generics for type-safe parameter handling.
// This function provides a flexible API that accepts different parameter types to construct
// user messages with various content combinations.
//
// Supported parameter types:
//   - string: Sets the text content of the user message
//   - []*content.Media: Sets media attachments for the message
//   - MessageParams: Complete parameter struct with text, media, and metadata fields
//
// The function uses type constraints and type switching to handle different input types,
// providing a convenient API for creating user messages with minimal boilerplate code.
//
// Examples:
//
//	NewUserMessage("Hello, how are you?")               // Creates message with text only
//	NewUserMessage(mediaSlice)                          // Creates message with media attachments only
//	NewUserMessage(MessageParams{                       // Creates message with full configuration
//	    Text: "What's in this image?",
//	    Media: mediaSlice,
//	    Metadata: map[string]any{"source": "mobile_app"},
//	})
//
// Note: User messages with media attachments are commonly used for multimodal AI interactions
// where the assistant needs to analyze images, process documents, or handle other media types.
func NewUserMessage[T string | []*content.Media | MessageParams](param T) *UserMessage {
	var p MessageParams

	switch typedParam := any(param).(type) {
	case string:
		p.Text = typedParam
	case []*content.Media:
		p.Media = typedParam
	case MessageParams:
		p = typedParam
	}

	if p.Media == nil {
		p.Media = make([]*content.Media, 0)
	}

	return &UserMessage{
		message: newMessage(p.Text, p.Metadata),
		media:   slices.Clone(p.Media),
	}
}
