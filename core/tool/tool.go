package tool

import (
	"context"
	"errors"

	"github.com/Tangerg/scope/core/chat"
)

var ErrInvalidTool = errors.New("tool: invalid tool")

// Tool is the minimal executable capability used by model-driven runtimes.
// Definition returns an independent snapshot safe to expose to a model. Call
// owns argument decoding and returns the text sent back as a tool result.
//
// Tool assigns no control-flow meaning to errors. Retry, pause, abort, and
// ordinary error feedback belong to the runtime driving the tool.
type Tool interface {
	// Definition returns a detached, valid schema snapshot. Callers may expose or
	// mutate the returned value without changing subsequent calls or execution.
	Definition() chat.ToolDefinition
	// Call executes one model-supplied JSON argument document. The implementation
	// owns strict decoding and capability-specific validation; ordinary tool
	// failure is returned as error without assigning retry or control-flow
	// meaning. Implementations must honor ctx and must not retain arguments.
	Call(ctx context.Context, arguments string) (string, error)
}
