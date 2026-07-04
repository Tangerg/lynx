package server

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// RuntimeServices is the accessor surface the protocol server needs from
// the runtime bundle. Defined here (consumer side) so the server depends
// on the narrow set of accessors it actually calls, not the concrete
// *internal/runtime.Runtime — which keeps the protocol layer free of an
// internal-package import and lets a future remote runtime (or a test
// fake) satisfy the surface without standing up the real bundle.
//
// *internal/runtime.Runtime satisfies this implicitly; the composition
// root (cmd/lyra) passes the concrete value where a RuntimeServices is
// expected.
type RuntimeServices interface {
	turnAccess
	sessionAccess
	transcriptAccess
	lifecycleStoreAccess
	toolAccess
	knowledgeAccess
	approvalAccess
	scheduleAccess
	providerAccess
	mcpAccess
	workspaceCatalogAccess
	hookAccess
	modelRoleAccess
	codebaseAccess
	maintenanceAccess
}

type turnAccess interface {
	Chat() turn.Service
}

type sessionAccess interface {
	Session() sessionsvc.Service
	DefaultModel() string
}

type transcriptAccess interface {
	Transcript() transcript.Store
}

type lifecycleStoreAccess interface {
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
	SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error
	MessageCount(ctx context.Context, sessionID string) (int, error)
	TruncateMessages(ctx context.Context, sessionID string, keepN int) error
	ForgetSession(sessionID string)
	RunInTx(ctx context.Context, fn func(context.Context) error) error
	Interrupts() interrupts.Store
}

type toolAccess interface {
	Tool() toolsvc.Service
}

type knowledgeAccess interface {
	Memory() knowledge.Service
}

type approvalAccess interface {
	Approval() approval.Service
}

type scheduleAccess interface {
	Schedules() schedule.Service
}

type providerAccess interface {
	Providers() providersvc.Service
	ProbeProvider(ctx context.Context, entry providersvc.Provider) error
	DefaultProvider() string
}

type mcpAccess interface {
	MCPServerStatuses() []kernel.McpServerStatus
	ReconnectMCPServer(ctx context.Context, name string) error
	AuthorizeMCPServer(ctx context.Context, name string) error
	MCPTools(ctx context.Context, server string) ([]kernel.McpToolInfo, error)
	MCPRegistry() mcpserver.Service
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

type modelRoleAccess interface {
	UtilityRole() (provider, model string)
	SetUtilityRole(ctx context.Context, provider, model string) error
	EmbeddingRole() (provider, model string)
	SetEmbeddingRole(ctx context.Context, provider, model string) error
}

type codebaseAccess interface {
	CodebaseIndex() codebaseindex.Service
}

type maintenanceAccess interface {
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}
