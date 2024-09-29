package message

func NewUserMessage(content string) *UserMessage {
	return &UserMessage{
		content:  content,
		metadata: make(map[string]any),
	}
}

type UserMessage struct {
	content  string
	metadata map[string]any
}

func (s *UserMessage) Role() Role {
	return User
}

func (s *UserMessage) Content() string {
	return s.content
}

func (s *UserMessage) Metadata() map[string]any {
	return s.metadata
}
