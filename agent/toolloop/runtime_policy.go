package toolloop

import (
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/tools"
)

type returnDirectMarker interface {
	ReturnsDirect() bool
}

// Direct marks a runtime Tool so a round consisting entirely of direct tools
// completes with its final ToolResult instead of making another model call.
// It preserves the tool-loop scheduling declaration but does not proxy
// host-specific optional interfaces; hosts must compose those capabilities
// outside this control-flow decorator.
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

func (directRuntimeTool) ReturnsDirect() bool { return true }

// ConcurrencyKey preserves the wrapped tool's optional scheduling contract.
// A policy decorator must not accidentally turn an isolated/read-only tool
// into an exclusive one.
func (t directRuntimeTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if capability, ok := t.Tool.(ConcurrentTool); ok {
		return capability.ConcurrencyKey(arguments)
	}
	return "", false
}

func returnsDirectRuntime(tool tools.Tool) (direct bool, err error) {
	marker, ok := tool.(returnDirectMarker)
	if !ok {
		return false, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool %T ReturnsDirect panicked", tool), recovered)
		}
	}()
	return marker.ReturnsDirect(), nil
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
