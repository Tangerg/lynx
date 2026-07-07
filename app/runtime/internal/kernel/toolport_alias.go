package kernel

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
	"github.com/Tangerg/lynx/core/model/chat"
)

const (
	ToolRoleCoding  = toolport.ToolRoleCoding
	ToolRoleSubtask = toolport.ToolRoleSubtask

	MCPTransportHTTP  = toolport.MCPTransportHTTP
	MCPTransportStdio = toolport.MCPTransportStdio
)

type (
	ToolResolver    = toolport.ToolResolver
	MCPTransport    = toolport.MCPTransport
	MCPToolInfo     = toolport.MCPToolInfo
	MCPServerStatus = toolport.MCPServerStatus
	McpToolInfo     = toolport.McpToolInfo
	McpServerStatus = toolport.McpServerStatus
	MCPServerConfig = toolport.MCPServerConfig
	MCPControl      = toolport.MCPControl
)

var ErrUnknownMCPServer = toolport.ErrUnknownMCPServer

type emptyToolResolver struct{}

func (*emptyToolResolver) Name() string { return "empty-tool-resolver" }

func (*emptyToolResolver) Resolve(context.Context, core.ToolGroupRequirement) (core.ToolGroup, error) {
	return nil, nil
}

func (*emptyToolResolver) SetTask(chat.Tool) {}
