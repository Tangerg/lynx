package message

func NewAssistantMessage(content string, metadata map[string]any, toolCalls ...*ToolCallRequest) *AssistantMessage {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[KeyOfMessageType] = Assistant.String()
	rv := &AssistantMessage{
		content:          content,
		metadata:         metadata,
		toolCallRequests: make([]*ToolCallRequest, 0, len(toolCalls)),
	}
	for _, toolCall := range toolCalls {
		if toolCall != nil {
			rv.toolCallRequests = append(rv.toolCallRequests, toolCall)
		}
	}
	return rv
}

var _ ChatMessage = (*AssistantMessage)(nil)

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
