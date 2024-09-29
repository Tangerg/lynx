package message

func NewAssisantMessage(content string) *AssisantMessage {
	return &AssisantMessage{
		content:  content,
		metadata: make(map[string]any),
	}
}

type AssisantMessage struct {
	content  string
	metadata map[string]any
}

func (s *AssisantMessage) Role() Role {
	return Assistant
}

func (s *AssisantMessage) Content() string {
	return s.content
}

func (s *AssisantMessage) Metadata() map[string]any {
	return s.metadata
}
