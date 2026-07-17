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
	if valueIsNil(tool) {
		return nil
	}
	return directRuntimeTool{Tool: tool}
}

type directRuntimeTool struct {
	tools.Tool
}

var _ tools.FileMutationReporter = directRuntimeTool{}

func (directRuntimeTool) ReturnsDirect() bool { return true }

func (t directRuntimeTool) Definition() chat.ToolDefinition {
	return t.Tool.Definition()
}

func (t directRuntimeTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.Tool.Call(ctx, arguments)
}

// ConcurrencyKey preserves the wrapped tool's optional scheduling contract.
// A policy decorator must not accidentally turn an isolated/read-only tool
// into an exclusive one.
func (t directRuntimeTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if capability, ok := t.Tool.(ConcurrentTool); ok {
		return capability.ConcurrencyKey(arguments)
	}
	return "", false
}

// MutationPaths preserves file-side-effect metadata through the control-flow
// decorator. Direct changes only what ends the loop; it must not hide which
// resources the underlying call may mutate.
func (t directRuntimeTool) MutationPaths(arguments string) ([]string, error) {
	if reporter, ok := t.Tool.(tools.FileMutationReporter); ok {
		return reporter.MutationPaths(arguments)
	}
	return nil, nil
}

func returnsDirectRuntime(tool tools.Tool) bool {
	direct, ok := tool.(returnDirectMarker)
	return ok && direct.ReturnsDirect()
}

func valueIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
