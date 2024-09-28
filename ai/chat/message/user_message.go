package message

type UserMessage struct {
	content  string
	metaData map[string]any
}

func NewUserMessage(content string) *UserMessage {
	return &UserMessage{
		content:  content,
		metaData: make(map[string]any),
	}
}

func (s *UserMessage) Role() Role {
	return User
}

func (s *UserMessage) Content() string {
	return s.content
}

func (s *UserMessage) Metadata() map[string]any {
	return s.metaData
}
