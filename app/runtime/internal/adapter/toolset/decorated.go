package toolset

import (
	"context"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/core/chat"
)

// decorate replaces Call while preserving the inner capability declarations
// — the shared spine of the read/edit guards and post-edit diagnostics.
func decorate(inner toolcontract.Tool, call func(ctx context.Context, arguments string) (string, error)) toolcontract.Tool {
	return &decorated{inner: inner, call: call}
}

// decorated is the backing type for [decorate]: it overrides Call while
// delegating Definition plus optional tool-loop declarations to the wrapped
// tool, so a decorator stack preserves the inner tool's full contract.
type decorated struct {
	inner toolcontract.Tool
	call  func(ctx context.Context, arguments string) (string, error)
}

func (d *decorated) Definition() chat.ToolDefinition { return d.inner.Definition() }

func (d *decorated) Call(ctx context.Context, arguments string) (string, error) {
	return d.call(ctx, arguments)
}

// Unwrap exposes the wrapped tool so its optional tool-loop declarations — a
// keyed file tool's per-path conflict class, where its edits land, a
// return-direct policy — survive the whole decorator stack. Only Call is
// overridden here.
func (d *decorated) Unwrap() toolcontract.Tool { return d.inner }
