package toolset

import (
	"context"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/core/chat"
)

// wrapTool returns a Tool that runs call while preserving inner's Definition
// — the shared spine of the tool toolMiddleware (read/edit guards, post-edit
// diagnostics). The result stands in for inner, so inner's optional tool-loop
// declarations survive the whole decorator stack.
func wrapTool(inner toolcontract.Tool, call func(ctx context.Context, arguments string) (string, error)) toolcontract.Tool {
	return &decoratedTool{inner: inner, call: call}
}

// decoratedTool is the backing type for [wrapTool]: it overrides Call while
// delegating Definition plus optional tool-loop declarations to the wrapped
// tool, so a stack of toolMiddleware preserves the inner tool's full contract.
type decoratedTool struct {
	inner toolcontract.Tool
	call  func(ctx context.Context, arguments string) (string, error)
}

func (d *decoratedTool) Definition() chat.ToolDefinition { return d.inner.Definition() }

func (d *decoratedTool) Call(ctx context.Context, arguments string) (string, error) {
	return d.call(ctx, arguments)
}

// Unwrap exposes the wrapped tool so its optional tool-loop declarations — a
// keyed file tool's per-path conflict class, where its edits land, a
// return-direct policy — survive the whole decorator stack. Only Call is
// overridden here.
func (d *decoratedTool) Unwrap() toolcontract.Tool { return d.inner }
