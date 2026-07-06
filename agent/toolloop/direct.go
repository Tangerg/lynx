package toolloop

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

type returnDirectMarker interface {
	ReturnsDirect() bool
}

type returnDirectTool struct {
	inner chat.Tool
}

// ReturnDirect marks a tool as terminating the loop with its result instead
// of feeding that result into another model round.
func ReturnDirect(tool chat.Tool) chat.Tool {
	if tool == nil {
		return nil
	}
	return returnDirectTool{inner: tool}
}

func returnsDirect(tool chat.Tool) bool {
	direct, ok := tool.(returnDirectMarker)
	return ok && direct.ReturnsDirect()
}

func (t returnDirectTool) Definition() chat.ToolDefinition {
	return t.inner.Definition()
}

func (t returnDirectTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.inner.Call(ctx, arguments)
}

func (t returnDirectTool) ReturnsDirect() bool {
	return true
}

func (t returnDirectTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if c, ok := t.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	}); ok {
		return c.ConcurrencyKey(arguments)
	}
	return "", false
}
