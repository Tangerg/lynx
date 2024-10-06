package message

type ToolCall struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}

type ToolResponse struct {
	ID   string
	Name string
	Data string
}

func NewToolMessage(resps []*ToolResponse) *ToolMessage {
	return &ToolMessage{
		responses: resps,
		metadata:  make(map[string]any),
	}
}

type ToolMessage struct {
	responses []*ToolResponse
	content   string
	metadata  map[string]any
}

func (t *ToolMessage) Role() Role {
	return Tool
}

func (t *ToolMessage) Content() string {
	return t.content
}

func (t *ToolMessage) Metadata() map[string]any {
	return t.metadata
}
