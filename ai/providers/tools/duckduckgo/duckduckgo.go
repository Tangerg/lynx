package duckduckgo

import (
	"github.com/Tangerg/lynx/ai/model/tool"
)

var _ tool.CallableTool = (*DuckDuckGo)(nil)

type DuckDuckGo struct {
	definition *tool.Definition
	metadata   *tool.Metadata
}

func (d *DuckDuckGo) Definition() *tool.Definition {
	return d.definition
}

func (d *DuckDuckGo) Metadata() *tool.Metadata {
	return d.metadata
}

func (d *DuckDuckGo) Call(ctx tool.Context, input string) (string, error) {
	//TODO implement me
	panic("implement me")
}
