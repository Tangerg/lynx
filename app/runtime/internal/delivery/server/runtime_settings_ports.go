package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// settingsUseCases groups runtime-wide configuration and management contexts:
// tools, memory, approval, providers, and model roles. Schedules moved to the
// application/schedules coordinator (delivery holds it directly).
type settingsUseCases interface {
	ListRegisteredTools(ctx context.Context) ([]toolsvc.Tool, error)
	InvokeRegisteredTool(ctx context.Context, name string, arguments string) (string, error)
	HasMemory() bool
	ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error)
	Memory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error)
	UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error
	ApprovalMode(ctx context.Context) (approval.Mode, error)
	SetApprovalMode(ctx context.Context, mode approval.Mode) error
	ListApprovalRules(ctx context.Context, sessionID string) ([]approval.Rule, error)
	ForgetApprovalRule(ctx context.Context, id string) error
	ListRegisteredProviders(ctx context.Context) ([]providersvc.Provider, error)
	RegisteredProvider(ctx context.Context, id string) (providersvc.Provider, bool, error)
	ConfigureProvider(ctx context.Context, entry providersvc.Provider) error
	ProbeProvider(ctx context.Context, entry providersvc.Provider) error
	SupportedProviders() []providersvc.Metadata
	ProviderMetadata(id string) (providersvc.Metadata, bool)
	DefaultProvider() string
	UtilityRole() (provider, model string)
	SetUtilityRole(ctx context.Context, provider, model string) error
	EmbeddingRole() (provider, model string)
	SetEmbeddingRole(ctx context.Context, provider, model string) error
}
