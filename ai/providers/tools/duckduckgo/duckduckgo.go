package duckduckgo

import (
	"context"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ chat.CallableTool = (*DuckDuckGo)(nil)

type DuckDuckGo struct {
	definition chat.ToolDefinition
	metadata   chat.ToolMetadata
}

func (d *DuckDuckGo) Definition() chat.ToolDefinition {
	return d.definition
}

func (d *DuckDuckGo) Metadata() chat.ToolMetadata {
	return d.metadata
}

func (d *DuckDuckGo) Call(ctx context.Context, arguments string) (string, error) {
	//TODO implement me
	panic("implement me")
}
