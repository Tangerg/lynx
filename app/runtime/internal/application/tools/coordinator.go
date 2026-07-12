// Package tools is the application coordinator for the runtime's diagnostic
// tool registry: listing every tool the runtime exposes and invoking one
// directly, outside a chat turn. A thin read/invoke surface over the domain tool
// registry the delivery tools.* handlers drive.
package tools

import (
	"context"

	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Coordinator drives the diagnostic tool registry.
type Coordinator struct {
	registry toolsvc.Registry
}

// New returns a Coordinator over the tool registry.
func New(registry toolsvc.Registry) *Coordinator {
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
