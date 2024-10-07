package message

func NewUserMessage(content string, metadata map[string]any) *UserMessage {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[KeyOfMessageType] = User.String()
	return &UserMessage{
		content:  content,
		metadata: metadata,
	}
}

type UserMessage struct {
	content  string
	metadata map[string]any
}

func (s *UserMessage) Type() Type {
	return User
}

func (s *UserMessage) Content() string {
	return s.content
}

func (s *UserMessage) Metadata() map[string]any {
	return s.metadata
}
