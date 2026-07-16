// Package toolcall carries one model-requested tool call across the private
// ToolLoop/runtime boundary. It is internal because the context value is an
// implementation detail, not an application-facing ambient capability.
package toolcall

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

type contextKey struct{}

type scopedCall struct {
	call      chat.ToolCall
	processID string
}

// Bind attaches call to ctx and scopes it to the current ProcessView, if any.
// A nested child replaces the ProcessView and therefore cannot accidentally
// inherit its parent's call identity.
func Bind(ctx context.Context, call chat.ToolCall) context.Context {
	value := scopedCall{call: call}
	if process := core.ProcessViewFrom(ctx); process != nil {
		value.processID = process.ID()
	}
	return context.WithValue(ctx, contextKey{}, value)
}

// FromContext returns the bound call when it belongs to the current process.
func FromContext(ctx context.Context) (chat.ToolCall, bool) {
	if ctx == nil {
		return chat.ToolCall{}, false
	}
	value, ok := ctx.Value(contextKey{}).(scopedCall)
	if !ok {
		return chat.ToolCall{}, false
	}
	process := core.ProcessViewFrom(ctx)
	if value.processID == "" {
		if process != nil {
			return chat.ToolCall{}, false
		}
	} else if process == nil || process.ID() != value.processID {
		return chat.ToolCall{}, false
	}
	return value.call, true
}
