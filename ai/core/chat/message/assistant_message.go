package message

func NewAssistantMessage(content string) *AssistantMessage {
	return &AssistantMessage{
		content:  content,
		metadata: make(map[string]any),
	}
}

type AssistantMessage struct {
	content  string
	metadata map[string]any
}

func (s *AssistantMessage) Role() Role {
	return Assistant
}

func (s *AssistantMessage) Content() string {
	return s.content
}

func (s *AssistantMessage) Metadata() map[string]any {
	return s.metadata
}
