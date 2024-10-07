package message

func NewAssistantMessage(content string, metadata map[string]any, toolCalls []*ToolCallRequest) *AssistantMessage {
	if toolCalls == nil {
		toolCalls = make([]*ToolCallRequest, 0)
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[KeyOfMessageType] = Assistant.String()
	return &AssistantMessage{
		toolCallRequests: toolCalls,
		content:          content,
		metadata:         metadata,
	}
}

type AssistantMessage struct {
	content          string
	metadata         map[string]any
	toolCallRequests []*ToolCallRequest
}

func (s *AssistantMessage) Type() Type {
	return Assistant
}

func (s *AssistantMessage) Content() string {
	return s.content
}

func (s *AssistantMessage) Metadata() map[string]any {
	return s.metadata
}

func (s *AssistantMessage) ToolCalls() []*ToolCallRequest {
	return s.toolCallRequests
}

func (s *AssistantMessage) HasToolCalls() bool {
	return len(s.toolCallRequests) > 0
}
