package kernel

import (
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
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
	MCPServerConfig = toolport.MCPServerConfig
	MCPControl      = toolport.MCPControl
)

var ErrUnknownMCPServer = toolport.ErrUnknownMCPServer
