package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/internal/toolcall"
	"github.com/Tangerg/lynx/core/chat"
)

// ToolCallFromContext returns the model-requested tool call currently being
// executed. The value is scoped to the current process, so a child process
// cannot accidentally observe its parent's call identity.
//
// Tools may use this for correlation and idempotency. Callers must treat the
// value as read-only; scheduling and resume ownership remain with the agent
// runtime.
func ToolCallFromContext(ctx context.Context) (chat.ToolCall, bool) {
	return toolcall.FromContext(ctx)
}
