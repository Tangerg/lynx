package toolloop

import (
	"reflect"

	"github.com/Tangerg/lynx/tool"
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

// InputlessContinuationTool is the optional capability required by
// [Runner.ContinuePaused]. It declares that a paused invocation may be
// re-entered after an internal durable dependency changes, without attaching
// external resume input. Tools that only pause for user input must not
// implement this capability.
type InputlessContinuationTool interface {
	CanContinueWithoutInput() bool
}

// Direct marks a runtime Tool so a round consisting entirely of direct tools
// completes with its final ToolResult instead of making another model call.
// It reports itself as a [tool.WrappingTool], so every optional capability of
// the tool it wraps — scheduling, deferral, a host's own — stays discoverable
// through it.
// Nil input remains nil and is rejected by a Registry or Runner.Run.
func Direct(tool tool.Tool) tool.Tool {
	if valueIsNil(tool) {
		return nil
	}
	return directRuntimeTool{Tool: tool}
}

type directRuntimeTool struct {
	tool.Tool
}

// Found by type assertion through the wrapping chain, so pin it.
var (
	_ DirectTool        = directRuntimeTool{}
	_ tool.WrappingTool = directRuntimeTool{}
)

func (directRuntimeTool) ReturnsDirect() bool { return true }

// Unwrap exposes the decorated tool so its optional capabilities remain
// discoverable; this decorator overrides only the direct-return marker.
func (t directRuntimeTool) Unwrap() tool.Tool { return t.Tool }

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
