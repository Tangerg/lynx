package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

// workspaceUseCases groups project-scoped capability management the runtime
// facade still owns: MCP live control + registry, and the semantic codebase
// index. Skills / recipes / hooks / memory moved to the workspace coordinator.
type workspaceUseCases interface {
	MCPServerStatuses() []toolport.MCPServerStatus
	MCPTools(ctx context.Context, server string) ([]toolport.MCPToolInfo, error)
	ReconnectMCPServer(ctx context.Context, name string) error
	AuthorizeMCPServer(ctx context.Context, name string) error
	ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error)
	MCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error)
	ConfigureMCPServer(ctx context.Context, srv mcpserver.Server) error
	RemoveMCPServer(ctx context.Context, name string) error
	SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error
	TestMCPServer(ctx context.Context, srv mcpserver.Server) error
	HasCodebaseIndex() bool
	SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error)
	CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error)
	StartCodebaseReindex(ctx context.Context, root string) error
}
