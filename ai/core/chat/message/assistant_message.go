package message

func NewAssistantMessage(content string) *AssistantMessage {
	return &AssistantMessage{
		toolCalls: make([]*ToolCall, 0),
		content:   content,
		metadata:  make(map[string]any),
	}
}

type AssistantMessage struct {
	toolCalls []*ToolCall
	content   string
	metadata  map[string]any
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

func (s *AssistantMessage) ToolCalls() []*ToolCall {
	return s.toolCalls
}

func (s *AssistantMessage) HasToolCalls() bool {
	return len(s.toolCalls) > 0
}
