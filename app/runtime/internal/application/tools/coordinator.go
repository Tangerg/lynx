// Package tools is the application coordinator for the runtime's diagnostic
// tool catalog: listing every tool the runtime exposes and invoking one
// directly, outside a chat turn.
package tools

import (
	"context"

	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Registry is the directly invocable diagnostic-tool catalog. It is deliberately
// distinct from the agent's full tool set: every entry must be safe to run
// outside a turn and must honor the supplied workspace root.
type Registry interface {
	List(ctx context.Context) ([]toolsvc.Tool, error)
	Invoke(ctx context.Context, root, name, arguments string) (toolsvc.Result, error)
}

// Roots resolves the workspace root an external diagnostic invocation is
// allowed to inspect. The workspace use case owns cwd admission; tools never
// accept an unchecked client path as their filesystem root.
type Roots interface {
	ResolveRoot(cwd string) (string, error)
}

// Invocation is one direct, read-only diagnostic tool call.
type Invocation struct {
	Name      string
	Arguments string
	Cwd       string
}

// Coordinator drives direct diagnostic-tool use cases.
type Coordinator struct {
	registry Registry
	roots    Roots
}

// New returns a Coordinator over the direct tool registry and workspace-root
// admission boundary.
func New(registry Registry, roots Roots) *Coordinator {
	return &Coordinator{registry: registry, roots: roots}
}

// List returns every tool that can be invoked directly outside a turn.
func (c *Coordinator) List(ctx context.Context) ([]toolsvc.Tool, error) {
	return c.registry.List(ctx)
}

// Invoke runs one direct diagnostic tool within its admitted workspace root.
func (c *Coordinator) Invoke(ctx context.Context, in Invocation) (toolsvc.Result, error) {
	root, err := c.roots.ResolveRoot(in.Cwd)
	if err != nil {
		return toolsvc.Result{}, err
	}
	return c.registry.Invoke(ctx, root, in.Name, in.Arguments)
}
