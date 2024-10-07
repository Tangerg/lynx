package message

// Type represents the message role of a participant in a conversation.
type Type string

func (r Type) String() string {
	return string(r)
}

func (r Type) IsSystem() bool {
	return r == System
}

func (r Type) IsUser() bool {
	return r == User
}

func (r Type) IsAssistant() bool {
	return r == Assistant
}

func (r Type) IsTool() bool {
	return r == Tool
}

// Constants defining the various roles in a chat system.
const (
	// System represents messages or instructions from the system itself.
	System Type = "system"

	// User represents messages from the end-user or client.
	User Type = "user"

	// Assistant represents messages from an AI assistant or chatbot.
	Assistant Type = "assistant"

	// Tool represents messages from integrated tools or external services.
	Tool Type = "tool"
)

const (
	// KeyOfMessageType for metadata use
	KeyOfMessageType = "message_type"
)
