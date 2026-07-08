package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

type ToolSource func(context.Context) ([]chat.Tool, error)

// MCPResolver adapts a tool source into a [core.ToolGroupResolver] extension.
type MCPResolver struct {
	name     string
	role     string
	tools    ToolSource
	metadata core.ToolGroupMetadata
}

// NewMCPResolver returns a resolver answering for role,
// delegating tool discovery to tools. Both arguments are required;
// nil/empty inputs return an error — caller decides whether to
// surface or panic.
func NewMCPResolver(role string, tools ToolSource) (*MCPResolver, error) {
	if role == "" {
		return nil, errors.New("runtime.NewMCPResolver: role must not be empty")
	}
	if tools == nil {
		return nil, errors.New("runtime.NewMCPResolver: tools must not be nil")
	}
	return &MCPResolver{
		name:     "mcp-tool-resolver:" + role,
		role:     role,
		tools:    tools,
		metadata: core.SimpleToolGroupMetadata{RoleText: role},
	}, nil
}

func (r *MCPResolver) Name() string { return r.name }

// Resolve returns a lazy [core.ToolGroup] for matching roles.
func (r *MCPResolver) Resolve(_ context.Context, req core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
	if req.Role != r.role {
		return nil, false, nil
	}
	return core.NewLazyToolGroup(r.metadata, r.tools), true, nil
}
