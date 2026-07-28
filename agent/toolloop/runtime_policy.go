package toolloop

import (
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/tools"
)

// DirectTool is the optional capability a tool implements to end the round it
// completes: a round consisting entirely of direct tools returns its final
// ToolResult instead of making another model call.
//
// Like [ConcurrentTool] and [DeferredTool] it is named here so a tool — or a
// host asking what a tool declared — states the intent without depending on a
// particular loop driver, and a driver that ignores the advice stays correct.
// [Direct] wraps a tool to declare it; implementing this reports the same thing
// directly.
type DirectTool interface {
	ReturnsDirect() bool
}

// Direct marks a runtime Tool so a round consisting entirely of direct tools
// completes with its final ToolResult instead of making another model call.
// It reports itself as a [tools.WrappingTool], so every optional capability of
// the tool it wraps — scheduling, deferral, a host's own — stays discoverable
// through it.
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

// Unwrap exposes the decorated tool so its optional capabilities remain
// discoverable; this decorator overrides only the direct-return marker.
func (t directRuntimeTool) Unwrap() tools.Tool { return t.Tool }

func returnsDirectRuntime(tool tools.Tool) (direct bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool %T direct-return lookup panicked", tool), recovered)
		}
	}()
	marker, ok, err := tools.Capability[DirectTool](tool)
	if err != nil {
		return false, fmt.Errorf("tool %T direct-return lookup: %w", tool, err)
	}
	if !ok {
		return false, nil
	}
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
