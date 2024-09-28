package message

type AssisantMessage struct {
	content  string
	metaData map[string]any
}

func (s *AssisantMessage) Role() Role {
	return Assistant
}

func (s *AssisantMessage) Content() string {
	return s.content
}

func (s *AssisantMessage) Metadata() map[string]any {
	return s.metaData
}
