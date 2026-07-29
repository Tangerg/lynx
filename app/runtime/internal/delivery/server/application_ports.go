package server

import (
	"context"
	"io"

	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	feedbackapp "github.com/Tangerg/lynx/app/runtime/internal/application/feedback"
	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	toolapp "github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/application/usage"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Every interface below is defined by Delivery — the consuming side. They keep
// the transport dependent on exactly the use cases it drives, not on concrete
// application coordinators or their unrelated methods.

type sessionUseCases interface {
	CreateView(ctx context.Context, title, cwd string) (sessions.SessionView, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ForkView(ctx context.Context, spec sessions.ForkSpec) (sessions.SessionView, error)
	ListViewPage(ctx context.Context, cursor string, limit int) (keyset.Page[sessions.SessionView], error)
	ExportSession(ctx context.Context, sessionID string) (sessions.ExportResult, error)
	RestorePortableSession(ctx context.Context, snapshot sessions.PortableSnapshot) (sessions.SessionView, error)
	Rollback(ctx context.Context, spec sessions.RollbackSpec) (sessions.RollbackResult, error)
	UpdateView(ctx context.Context, id string, patch session.Patch) (sessions.SessionView, error)
	View(ctx context.Context, id string) (sessions.SessionView, error)
}

type integrationUseCases interface {
	AuthorizeMCPServer(ctx context.Context, name string) error
	ConfigureMCPServer(ctx context.Context, input integrations.MCPServerInput) (integrations.MCPServerConfig, error)
	ListMCPServerConfigs(ctx context.Context) ([]integrations.MCPServerConfig, error)
	MCPServerStatuses(ctx context.Context) []integrations.MCPServerStatus
	MCPTools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error)
	ReconnectMCPServer(ctx context.Context, name string) error
	RemoveMCPServer(ctx context.Context, name string) error
	SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error
	TestMCPServer(ctx context.Context, input integrations.MCPServerInput) (integrations.MCPTestResult, error)
}

type approvalUseCases interface {
	ForgetRule(ctx context.Context, id string) error
	ListRules(ctx context.Context, sessionID string) ([]approval.Rule, error)
	Mode(ctx context.Context) (approval.Mode, error)
	SetMode(ctx context.Context, mode approval.Mode) error
}

type modelUseCases interface {
	ConfigureProvider(ctx context.Context, cmd models.ConfigureProviderCommand) (models.ProviderInfo, error)
	EmbeddingRole() modelref.Selection
	ListModels(ctx context.Context, providerID string) []models.Model
	ListProviders(ctx context.Context) ([]models.ProviderInfo, error)
	SetEmbeddingRole(ctx context.Context, providerID, model string) (modelref.Selection, error)
	SetUtilityRole(ctx context.Context, provider, model string) (modelref.Selection, error)
	TestProvider(ctx context.Context, id string) (models.ProviderTestOutcome, error)
	UtilityRole() modelref.Selection
}

type toolUseCases interface {
	Invoke(ctx context.Context, in toolapp.Invocation) (toolsvc.Result, error)
	List(ctx context.Context) ([]toolsvc.Tool, error)
}

type codebaseUseCases interface {
	Available() bool
	Search(ctx context.Context, cwd, query string, limit int) ([]codebaseindex.Hit, error)
	StartReindex(ctx context.Context, cwd string) (string, error)
	Status(ctx context.Context, cwd string) (codebase.Status, error)
}

type runUseCases interface {
	Cancel(ctx context.Context, cmd runs.CancelCommand) error
	Resume(ctx context.Context, cmd runs.ResumeCommand) (runs.StartResult, error)
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
	Steer(ctx context.Context, cmd runs.SteerCommand) error
	Subscribe(ctx context.Context, req runs.SubscribeRequest) (runs.Subscription, error)
	// ReplayRetention is what discovery publishes. Reading it from the enforcer is
	// the point: a limit the client is told and a limit the runtime evicts by must
	// be one number, or discovery is describing a runtime that does not exist.
	ReplayRetention() runs.Retention
}

type queryUseCases interface {
	ListItemPage(ctx context.Context, scope queries.ItemScope, order transcript.SequenceOrder, cursor string, limit int) (queries.ItemPage, error)
	ListPendingInterruptPage(ctx context.Context, sessionID, rootRunID string, caller execution.RunProtocolProfile, cursor string, limit int) (keyset.Page[interrupts.Pending], error)
	Run(ctx context.Context, runID string) (transcript.Run, bool, error)
	TodoState(ctx context.Context, sessionID string) (todo.State, error)
	ListRunPage(ctx context.Context, sessionID string, statuses []execution.RunStatus, cursor string, limit int) (keyset.Page[transcript.Run], error)
}

type usageUseCases interface {
	Session(ctx context.Context, sessionID string) (usage.SessionReport, error)
	Summary(ctx context.Context, sinceDays int) (usage.Summary, error)
}

type feedbackUseCases interface {
	Record(ctx context.Context, command feedbackapp.Command) error
}

type scheduleManagementUseCases interface {
	Available() bool
	Create(ctx context.Context, cmd schedules.CreateCommand) (schedule.Schedule, error)
	Delete(ctx context.Context, id string) error
	ListPage(ctx context.Context, cursor string, limit int) (keyset.Page[schedule.Schedule], error)
	Update(ctx context.Context, cmd schedules.UpdateCommand) (schedule.Schedule, error)
}

type scheduleFiringUseCases interface {
	Available() bool
	RunNow(ctx context.Context, id string) (schedules.RunHandle, error)
}

type workspaceFileUseCases interface {
	FileHead(ctx context.Context, cwd, path string, lines int) (workspaceapp.FileHead, error)
	Grep(ctx context.Context, cwd string, input workspaceapp.GrepInput) (workspaceapp.GrepResult, error)
	ListFiles(ctx context.Context, input workspaceapp.FileListInput) (workspaceapp.FilePage, error)
	ReadFile(ctx context.Context, cwd string, input workspaceapp.FileReadInput) (workspaceapp.FileReadResult, error)
}

type workspaceVCSUseCases interface {
	Diff(ctx context.Context, input workspaceapp.DiffInput) (workspaceapp.Diff, error)
	ListFileChanges(ctx context.Context, cwd string) ([]workspaceapp.FileChange, error)
}

type workspaceDiscoveryUseCases interface {
	ListAgentDocs(ctx context.Context, cwd string) ([]workspaceapp.AgentDoc, error)
	ListProjects(ctx context.Context) ([]workspaceapp.Project, error)
	ListRecipes(ctx context.Context, cwd string) ([]workspaceapp.Recipe, error)
}

type workspaceKnowledgeUseCases interface {
	HasMemory() bool
	ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error)
	Memory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error)
	UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error
}

type workspaceSkillUseCases interface {
	ArchiveSkill(ctx context.Context, name string) error
	ListManagedSkills(ctx context.Context) ([]skills.Entry, error)
	ListSkillDrafts(ctx context.Context) ([]skills.DraftInfo, error)
	ListSkills(ctx context.Context, cwd string) ([]workspaceapp.SkillInfo, error)
	PromoteSkillDraft(ctx context.Context, handle skills.DraftHandle) error
	RejectSkillDraft(ctx context.Context, handle skills.DraftHandle) error
	RestoreSkill(ctx context.Context, name string) error
}

type workspaceHookUseCases interface {
	InspectHooks(ctx context.Context, cwd string) (workspaceapp.HookInspection, error)
	SetProjectHookTrust(ctx context.Context, projectRoot string, trusted bool) error
}

type workspaceWatchUseCases interface {
	HasFileWatch() bool
	WatchGitState(cwds []string, notify func()) (io.Closer, error)
}
