package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

type mcpAccess interface {
	MCPServerStatuses() []kernel.MCPServerStatus
	ReconnectMCPServer(ctx context.Context, name string) error
	AuthorizeMCPServer(ctx context.Context, name string) error
	MCPTools(ctx context.Context, server string) ([]kernel.MCPToolInfo, error)
	ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error)
	MCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error)
	ConfigureMCPServer(ctx context.Context, srv mcpserver.Server) error
	RemoveMCPServer(ctx context.Context, name string) error
	SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error
	TestMCPServer(ctx context.Context, srv mcpserver.Server) error
}

type workspaceCatalogAccess interface {
	ListSkills(ctx context.Context, cwd string) ([]kernel.SkillInfo, error)
	ListRecipes(ctx context.Context, cwd string) ([]recipes.Recipe, error)
}

type hookAccess interface {
	InspectHooks(ctx context.Context, cwd string) hooks.Inspection
	SetProjectHookTrust(ctx context.Context, projectRoot string, trusted bool) error
}

type codebaseAccess interface {
	HasCodebaseIndex() bool
	SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error)
	CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error)
	StartCodebaseReindex(ctx context.Context, root string) error
}
