package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

type mcpStatusAccess interface {
	MCPServerStatuses() []toolport.MCPServerStatus
}

type mcpToolCatalogAccess interface {
	MCPTools(ctx context.Context, server string) ([]toolport.MCPToolInfo, error)
}

type mcpReconnectAccess interface {
	ReconnectMCPServer(ctx context.Context, name string) error
}

type mcpAuthorizationAccess interface {
	AuthorizeMCPServer(ctx context.Context, name string) error
}

type mcpRegistryListAccess interface {
	ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error)
}

type mcpRegistryReadAccess interface {
	MCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error)
}

type mcpRegistryConfigureAccess interface {
	ConfigureMCPServer(ctx context.Context, srv mcpserver.Server) error
}

type mcpRegistryRemoveAccess interface {
	RemoveMCPServer(ctx context.Context, name string) error
}

type mcpRegistryEnableAccess interface {
	SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error
}

type mcpRegistryProbeAccess interface {
	TestMCPServer(ctx context.Context, srv mcpserver.Server) error
}

type skillCatalogAccess interface {
	ListSkills(ctx context.Context, cwd string) ([]kernel.SkillInfo, error)
}

type recipeCatalogAccess interface {
	ListRecipes(ctx context.Context, cwd string) ([]recipes.Recipe, error)
}

type hookInspectionAccess interface {
	InspectHooks(ctx context.Context, cwd string) hooks.Inspection
}

type hookTrustAccess interface {
	SetProjectHookTrust(ctx context.Context, projectRoot string, trusted bool) error
}

type codebaseAvailabilityAccess interface {
	HasCodebaseIndex() bool
}

type codebaseSearchAccess interface {
	SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error)
}

type codebaseStatusAccess interface {
	CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error)
}

type codebaseReindexAccess interface {
	StartCodebaseReindex(ctx context.Context, root string) error
}
