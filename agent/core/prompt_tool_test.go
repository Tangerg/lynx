package core_test

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/core/chat"
)

type promptTool struct {
	name string
}

func (t promptTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        t.name,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (promptTool) Call(context.Context, string) (string, error) {
	return "", nil
}
