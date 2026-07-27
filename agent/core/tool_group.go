package core

import (
	"context"

	"github.com/Tangerg/lynx/tools"
)

// ToolGroup describes and supplies one resolved set of tools. Implementations
// own loading, caching, retry, synchronization, and lifecycle policy. Runtime
// may call Tools concurrently and does not retain or coordinate an
// implementation's mutable state.
type ToolGroup interface {
	Tools(ctx context.Context) ([]tools.Tool, error)
}

// ToolGroupResolver maps an abstract role to a concrete group. Registered
// as an engine extension; the runtime walks every registered resolver
// in registration order and the first one returning a non-nil group
// wins. Resolvers double as [Extension] so the dispatch site can
// attribute hits / errors by Name. Panics from Resolve or from the returned
// group's Tools method become attributed resolution errors. Valid at engine
// and process scope. Implementations own source discovery, caching,
// synchronization, retry, and connection lifecycle; Runtime only validates
// action role declarations and consumes the returned capability.
type ToolGroupResolver interface {
	Extension

	Resolve(ctx context.Context, role string) (ToolGroup, bool, error)
}
