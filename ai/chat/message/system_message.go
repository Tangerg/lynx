package message

type SystemMessage struct {
	content  string
	metadata map[string]any
}

func (s *SystemMessage) Role() Role {
	return System
}

func (s *SystemMessage) Content() string {
	return s.content
}

func (s *SystemMessage) Metadata() map[string]any {
	return s.metadata
}
