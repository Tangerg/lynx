package toolloop

import (
	"context"
	"reflect"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type returnDirectMarker interface {
	ReturnsDirect() bool
}

// Direct marks a runtime Tool so a round consisting entirely of direct tools
// completes with its final ToolResult instead of making another model call.
// Nil input remains nil and is rejected by tools.Registry or Runner.Run.
func Direct(tool tools.Tool) tools.Tool {
	if nilRuntimeTool(tool) {
		return nil
	}
	return directRuntimeTool{Tool: tool}
}

type directRuntimeTool struct {
	tools.Tool
}

func (directRuntimeTool) ReturnsDirect() bool { return true }

func (t directRuntimeTool) Definition() chat.ToolDefinition {
	return t.Tool.Definition()
}

func (t directRuntimeTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.Tool.Call(ctx, arguments)
}

func returnsDirectRuntime(tool tools.Tool) bool {
	direct, ok := tool.(returnDirectMarker)
	return ok && direct.ReturnsDirect()
}

func nilRuntimeTool(tool tools.Tool) bool {
	if tool == nil {
		return true
	}
	value := reflect.ValueOf(tool)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
