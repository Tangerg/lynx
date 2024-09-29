package message

// Role represents the role of a participant in a conversation.
type Role string

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
