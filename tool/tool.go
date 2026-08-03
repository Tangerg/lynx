package tool

import (
	"context"
	"errors"
	"reflect"

	"github.com/Tangerg/lynx/core/chat"
)

// ErrInvalidTool reports a nil tool or invalid model-visible definition.
var ErrInvalidTool = errors.New("tool: invalid tool")

// Tool is the minimal executable capability used by model-driven runtimes.
// Definition returns an independent snapshot safe to expose to a model. Call
// owns argument decoding and returns the text sent back as a tool result.
//
// Tool assigns no control-flow meaning to errors. Retry, pause, abort, and
// ordinary error feedback belong to the runtime driving the tool.
type Tool interface {
	Definition() chat.ToolDefinition
	Call(ctx context.Context, arguments string) (string, error)
}

func nilTool(value Tool) bool {
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
