package tools

import (
	"context"

	"github.com/Tangerg/lynx/core/chat"
)

// Tool is the minimal executable capability used by an agent tool loop.
// Definition returns an independent snapshot safe to expose to a model. Call
// owns argument decoding and returns the text that should be sent back as a
// tool result.
//
// Call does not assign control-flow meaning to errors. Retry, pause, abort, and
// ordinary error feedback are policies of the runtime driving the tool.
type Tool interface {
	Definition() chat.ToolDefinition
	Call(ctx context.Context, arguments string) (string, error)
}
