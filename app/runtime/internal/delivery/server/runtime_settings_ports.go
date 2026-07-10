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

// settingsUseCases groups runtime-wide configuration and management contexts:
// tools, memory, approval, schedules, providers, and model roles.
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
	ListSchedules(ctx context.Context) ([]schedule.Schedule, error)
	Schedule(ctx context.Context, id string) (schedule.Schedule, error)
	CreateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
	UpdateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
	RecordScheduleRun(ctx context.Context, id string, ranAt time.Time) error
	RunScheduleWorker(ctx context.Context, runner schedule.Runner)
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
