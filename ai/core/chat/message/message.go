package message

import "github.com/Tangerg/lynx/ai/core/model"

// ChatMessage is an interface that defines the structure of a message in the chat system.
// It provides methods to access the role, content, and metadata of a message.
type ChatMessage interface {
	model.Content
	// Type returns the role of the message sender.
	Type() Type
}
