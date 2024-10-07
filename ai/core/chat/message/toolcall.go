package message

type ToolCallRequest struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}

type ToolCallResponse struct {
	ID   string
	Name string
	Data string
}
