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
	MCPStatusReader = toolport.MCPStatusReader
	MCPToolCatalog  = toolport.MCPToolCatalog

	MCPConnectionCommands = toolport.MCPConnectionCommands
	MCPRegistryCommands   = toolport.MCPRegistryCommands
)

var ErrUnknownMCPServer = toolport.ErrUnknownMCPServer
