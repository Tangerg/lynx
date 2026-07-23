// Package tools is the application coordinator for the runtime's diagnostic
// tool catalog: listing every tool the runtime exposes and invoking one
// directly, outside a chat turn.
package tools

import (
	"context"

	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Registry is the diagnostic catalog and invocation surface these use cases
// consume.
type Registry interface {
	List(ctx context.Context) ([]toolsvc.Tool, error)
	Invoke(ctx context.Context, name string, arguments string) (string, error)
}

// Coordinator drives diagnostic tool use cases.
type Coordinator struct {
	registry Registry
}

// New returns a Coordinator over the tool registry.
func New(registry Registry) *Coordinator {
	return &Coordinator{registry: registry}
}

// List returns every tool the runtime exposes for direct diagnostic invocation.
func (c *Coordinator) List(ctx context.Context) ([]toolsvc.Tool, error) {
	return c.registry.List(ctx)
}

// Invoke runs one registered tool directly outside a chat turn.
func (c *Coordinator) Invoke(ctx context.Context, name string, arguments string) (string, error) {
	return c.registry.Invoke(ctx, name, arguments)
}
