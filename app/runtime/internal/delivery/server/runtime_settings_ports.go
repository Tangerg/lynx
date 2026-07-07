package server

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type toolAccess interface {
	ListRegisteredTools(ctx context.Context) ([]toolsvc.Tool, error)
	InvokeRegisteredTool(ctx context.Context, name string, arguments string) (string, error)
}

type knowledgeAccess interface {
	HasMemory() bool
	ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error)
	Memory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error)
	UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error
}

type approvalAccess interface {
	ApprovalMode(ctx context.Context) (approval.Mode, error)
	SetApprovalMode(ctx context.Context, mode approval.Mode) error
	ListApprovalRules(ctx context.Context, sessionID string) ([]approval.Rule, error)
	ForgetApprovalRule(ctx context.Context, id string) error
}

type scheduleCatalogAccess interface {
	ListSchedules(ctx context.Context) ([]schedule.Schedule, error)
	Schedule(ctx context.Context, id string) (schedule.Schedule, error)
}

type scheduleMutationAccess interface {
	CreateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
	UpdateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
}

type scheduleRunRecorderAccess interface {
	RecordScheduleRun(ctx context.Context, id string, ranAt time.Time) error
}

type scheduleWorkerAccess interface {
	RunScheduleWorker(ctx context.Context, runner schedule.Runner)
}

type providerRegistryAccess interface {
	ListRegisteredProviders(ctx context.Context) ([]providersvc.Provider, error)
	RegisteredProvider(ctx context.Context, id string) (providersvc.Provider, bool, error)
	ConfigureProvider(ctx context.Context, entry providersvc.Provider) error
	ProbeProvider(ctx context.Context, entry providersvc.Provider) error
}

type providerCatalogAccess interface {
	SupportedProviders() []providersvc.Metadata
	ProviderMetadata(id string) (providersvc.Metadata, bool)
}

type providerDefaultAccess interface {
	DefaultProvider() string
}

type modelRoleAccess interface {
	UtilityRole() (provider, model string)
	SetUtilityRole(ctx context.Context, provider, model string) error
	EmbeddingRole() (provider, model string)
	SetEmbeddingRole(ctx context.Context, provider, model string) error
}
