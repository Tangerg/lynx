package server

import (
	"context"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	feedbackapp "github.com/Tangerg/lynx/app/runtime/internal/application/feedback"
	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Every interface below is defined by Delivery — the consuming side. They keep
// the protocol boundary dependent on exactly the use cases it drives.

type sessionUseCases interface {
	CreateView(ctx context.Context, title, cwd string) (sessions.View, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ForkView(ctx context.Context, spec sessions.ForkSpec) (sessions.View, error)
	ListViewPage(ctx context.Context, cursor string, limit int) (keyset.Page[sessions.View], error)
	ExportSession(ctx context.Context, sessionID string) (sessions.ExportResult, error)
	RestorePortableSession(ctx context.Context, snapshot sessions.PortableSnapshot) (sessions.View, error)
	Rollback(ctx context.Context, spec sessions.RollbackSpec) (sessions.RollbackResult, error)
	UpdateView(ctx context.Context, id string, patch session.Patch) (sessions.View, error)
	View(ctx context.Context, id string) (sessions.View, error)
}

type mcpUseCases interface {
	CreateAuthorizationAttempt(ctx context.Context, name string) (mcpapp.AuthorizationAttempt, error)
	CreateServer(ctx context.Context, input mcpapp.ServerInput) (mcpapp.Server, error)
	DeleteServer(ctx context.Context, name string) error
	AuthorizationAttempt(ctx context.Context, id string) (mcpapp.AuthorizationAttempt, error)
	AuthorizationAttemptRetention() time.Duration
	Servers(ctx context.Context) ([]mcpapp.Server, error)
	Tools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error)
	ReconnectServer(ctx context.Context, name string) error
	TestServer(ctx context.Context, input mcpapp.ServerInput) (mcpapp.TestResult, error)
	UpdateServer(ctx context.Context, name string, patch mcpapp.ServerPatch) (mcpapp.Server, error)
}

type approvalUseCases interface {
	ForgetRule(ctx context.Context, id string) error
	ListRules(ctx context.Context, sessionID string) ([]approval.Rule, error)
	DefaultMode(ctx context.Context) (approval.Mode, error)
	SetDefaultMode(ctx context.Context, mode approval.Mode) error
}

type modelUseCases interface {
	UpdateProvider(ctx context.Context, cmd models.UpdateProviderCommand) (models.ProviderInfo, error)
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
	Cancel(ctx context.Context, cmd runs.CancelCommand) (runs.CancelResult, error)
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
	ListPendingInterruptPage(ctx context.Context, sessionID, rootRunID string, caller execution.RunCapabilities, cursor string, limit int) (keyset.Page[interrupts.Pending], error)
	Run(ctx context.Context, runID string) (transcript.Run, bool, error)
	PlanState(ctx context.Context, sessionID string) (plan.State, error)
	ListRunPage(ctx context.Context, filter queries.RunPageFilter, cursor string, limit int) (keyset.Page[transcript.Run], error)
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
	Head(ctx context.Context, cwd, path string, lines int) (workspaceapp.FileHead, error)
	Grep(ctx context.Context, cwd string, input workspaceapp.GrepInput) (workspaceapp.GrepResult, error)
	List(ctx context.Context, input workspaceapp.FileListInput) (workspaceapp.FilePage, error)
	Read(ctx context.Context, cwd string, input workspaceapp.FileReadInput) (workspaceapp.FileReadResult, error)
}

type workspaceVCSUseCases interface {
	Diff(ctx context.Context, input workspaceapp.DiffInput) (workspaceapp.Diff, error)
	Changes(ctx context.Context, cwd string) ([]workspaceapp.FileChange, error)
}

type workspaceDiscoveryUseCases interface {
	AgentDocs(ctx context.Context, cwd string) ([]workspaceapp.AgentDoc, error)
	Workspaces(ctx context.Context) ([]workspaceapp.Summary, error)
	Recipes(ctx context.Context, cwd string) ([]workspaceapp.Recipe, error)
	Resolve(path string) (workspaceapp.Resolved, error)
}

type workspaceKnowledgeUseCases interface {
	Available() bool
	Entries(ctx context.Context, cwd string) ([]knowledge.Entry, error)
	Read(ctx context.Context, scope knowledge.Scope, cwd string) (string, error)
	Update(ctx context.Context, scope knowledge.Scope, cwd string, content string) error
}

type workspaceSkillUseCases interface {
	Archive(ctx context.Context, name string) error
	Managed(ctx context.Context) ([]skills.Entry, error)
	Proposals(ctx context.Context, cwd string) ([]skills.ProposalInfo, error)
	List(ctx context.Context, cwd string) ([]workspaceapp.SkillInfo, error)
	ApproveProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error
	RejectProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error
	Restore(ctx context.Context, name string) error
}

type workspaceHookUseCases interface {
	Inspect(ctx context.Context, cwd string) (workspaceapp.HookInspection, error)
	SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error
}

type workspaceWatchUseCases interface {
	Available() bool
	Watch(cwds []string, notify func()) (io.Closer, error)
}
