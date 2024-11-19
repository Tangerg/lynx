package message

func NewToolCallsMessage(resps []*ToolCallResponse, metadata map[string]any) *ToolCallsMessage {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[KeyOfMessageType] = Tool.String()
	return &ToolCallsMessage{
		responses: resps,
		metadata:  metadata,
	}
}

var _ ChatMessage = (*ToolCallsMessage)(nil)

type ToolCallsMessage struct {
	responses []*ToolCallResponse
	content   string
	metadata  map[string]any
}

func (t *ToolCallsMessage) Type() Type {
	return Tool
}

func (t *ToolCallsMessage) Content() string {
	return t.content
}

func (t *ToolCallsMessage) Metadata() map[string]any {
	return t.metadata
}

func (t *ToolCallsMessage) Responses() []*ToolCallResponse {
	return t.responses
}
