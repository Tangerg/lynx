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

// NewUserMessage creates a new user message with the given text content and media attachments.
//
// The media parameter can be nil or an empty slice if no media is needed.
//
// Optionally accepts metadata as a map. If multiple metadata maps are provided,
// only the first one will be used.
func NewUserMessage(text string, media []*content.Media, metadata ...map[string]any) *UserMessage {
	if media == nil {
		media = make([]*content.Media, 0)
	}
	return &UserMessage{
		message: newmessage(User, text, metadata...),
		media:   media,
	}
}
