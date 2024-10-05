package message

// Role represents the role of a participant in a conversation.
type Role string

func (r Role) String() string {
	return string(r)
}

func (r Role) IsSystem() bool {
	return r == System
}

func (r Role) IsUser() bool {
	return r == User
}

func (r Role) IsAssistant() bool {
	return r == Assistant
}

func (r Role) IsTool() bool {
	return r == Tool
}

// Constants defining the various roles in a chat system.
const (
	// System represents messages or instructions from the system itself.
	System Role = "system"

	// User represents messages from the end-user or client.
	User Role = "user"

	// Assistant represents messages from an AI assistant or chatbot.
	Assistant Role = "assistant"

	// Tool represents messages from integrated tools or external services.
	Tool Role = "tool"
)
