package message

// ChatMessage is an interface that defines the structure of a message in the chat system.
// It provides methods to access the role, content, and metadata of a message.
type ChatMessage interface {
	// Type returns the role of the message sender.
	Type() Type

	// Content returns the text content of the message.
	Content() string

	// Metadata returns additional information about the message as key-value pairs.
	Metadata() map[string]any
}
