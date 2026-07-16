package toolset

import (
	"context"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// wrapTool returns a Tool that runs call while preserving inner's Definition
// — the shared spine of the tool toolMiddleware (read/edit guards, post-edit
// diagnostics). It also forwards inner's optional tool-loop declarations so a
// keyed file tool's per-path conflict class and return-direct policy survive
// the whole decorator stack.
func wrapTool(inner tools.Tool, call func(ctx context.Context, arguments string) (string, error)) tools.Tool {
	return &decoratedTool{inner: inner, call: call}
}

// decoratedTool is the backing type for [wrapTool]: it overrides Call while
// delegating Definition plus optional tool-loop declarations to the wrapped
// tool, so a stack of toolMiddleware preserves the inner tool's full contract.
type decoratedTool struct {
	inner tools.Tool
	call  func(ctx context.Context, arguments string) (string, error)
}

func (d *decoratedTool) Definition() chat.ToolDefinition { return d.inner.Definition() }

func (d *decoratedTool) Call(ctx context.Context, arguments string) (string, error) {
	return d.call(ctx, arguments)
}

// ConcurrencyKey forwards the wrapped tool's concurrency declaration (matched
// structurally so this package needn't import the loop driver), so a keyed file
// tool keeps its per-path key through the decorator stack. A wrapped tool that
// declares nothing is exclusive (concurrent=false).
func (d *decoratedTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if c, ok := d.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	}); ok {
		return c.ConcurrencyKey(arguments)
	}
	return "", false
}

func (d *decoratedTool) ReturnsDirect() bool {
	if direct, ok := d.inner.(interface{ ReturnsDirect() bool }); ok {
		return direct.ReturnsDirect()
	}
	return false
}

func (d *decoratedTool) MutatedPaths(arguments string) ([]string, error) {
	if paths, ok := d.inner.(mutatedPathReporter); ok {
		return paths.MutatedPaths(arguments)
	}
	return nil, nil
}
