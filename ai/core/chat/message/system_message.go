package message

func NewSystemMessage(content string, metadata map[string]any) *SystemMessage {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[KeyOfMessageType] = System.String()
	return &SystemMessage{
		content:  content,
		metadata: metadata,
	}
}

type SystemMessage struct {
	content  string
	metadata map[string]any
}

func (s *SystemMessage) Type() Type {
	return System
}

func (s *SystemMessage) Content() string {
	return s.content
}

func (s *SystemMessage) Metadata() map[string]any {
	return s.metadata
}
