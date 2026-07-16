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

// FileMutationReporter is an optional tool capability for calls that may
// change files. MutationPaths derives the prospective targets from the same
// JSON arguments passed to Call. Runtimes use it for path locking and may
// publish the paths only after Call succeeds.
//
// It remains separate from Tool so read-only and non-filesystem tools keep the
// smallest useful contract.
type FileMutationReporter interface {
	MutationPaths(arguments string) ([]string, error)
}
