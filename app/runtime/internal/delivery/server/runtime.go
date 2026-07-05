package server

import (
	"context"
	"iter"
	"time"

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
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
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
	lifecycleAccess
	runSegmentAccess
	historyAccess
	interruptQueryAccess
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
	StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error)
	TurnEvents(ctx context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error)
	InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error
	ResumeTurn(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error
	RehydrateTurn(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error)
	CancelTurn(ctx context.Context, handle turn.TurnHandle) error
	TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error)
	SetTurnInterruptKinds(kinds []string)
}

type sessionAccess interface {
	ListSessions(ctx context.Context) ([]sessionsvc.Session, error)
	GetSession(ctx context.Context, id string) (sessionsvc.Session, error)
	CreateSession(ctx context.Context, title, cwd string) (sessionsvc.Session, error)
	DeleteSession(ctx context.Context, id string) error
	RenameSession(ctx context.Context, id, title string) error
	SetSessionModel(ctx context.Context, id, model string) error
	SetSessionCwd(ctx context.Context, id, cwd string) error
	SetSessionMetadata(ctx context.Context, id string, meta map[string]any) error
	SetSessionFavorite(ctx context.Context, id string, favorite bool) error
	DefaultModel() string
}

type transcriptAccess interface {
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
	ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
}

type historyAccess interface {
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
}

type lifecycleAccess interface {
	ClaimRunSlot(ctx context.Context, claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
	ClaimMutationSlot(claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error)
	ClaimResumeSlot(ctx context.Context, claims lifecycle.SessionClaimer, parentRunID string) (interrupts.Pending, lifecycle.RunAdmission, error)
	CancelParkedRun(ctx context.Context, runID string) error
	CancelRunTurn(ctx context.Context, run lifecycle.RunTurn) error
	ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution) (lifecycle.ResumedInterrupt, error)
	RollbackResolved(ctx context.Context, sessionID string, boundary lifecycle.RollbackBoundary) error
	ForkSession(ctx context.Context, spec lifecycle.ForkSpec) (sessionsvc.Session, error)
	RestoreSession(ctx context.Context, ses sessionsvc.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error
}

type runSegmentAccess interface {
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
}

type interruptQueryAccess interface {
	ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
}

type toolAccess interface {
	ListRegisteredTools(ctx context.Context) ([]toolsvc.Tool, error)
	InvokeRegisteredTool(ctx context.Context, name string, arguments string) (string, error)
}

type knowledgeAccess interface {
	HasMemory() bool
	ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error)
	GetMemory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error)
	UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error
}

type approvalAccess interface {
	GetApprovalMode(ctx context.Context) (approval.Mode, error)
	SetApprovalMode(ctx context.Context, mode approval.Mode) error
	ListApprovalRules(ctx context.Context, sessionID string) ([]approval.Rule, error)
	ForgetApprovalRule(ctx context.Context, id string) error
}

type scheduleAccess interface {
	ListSchedules(ctx context.Context) ([]schedule.Schedule, error)
	GetSchedule(ctx context.Context, id string) (schedule.Schedule, error)
	CreateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
	UpdateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
	RecordScheduleRun(ctx context.Context, id string, ranAt time.Time) error
	RunScheduleWorker(ctx context.Context, runner schedule.Runner)
}

type providerAccess interface {
	ListRegisteredProviders(ctx context.Context) ([]providersvc.Provider, error)
	GetRegisteredProvider(ctx context.Context, id string) (providersvc.Provider, bool, error)
	ConfigureProvider(ctx context.Context, entry providersvc.Provider) error
	ProbeProvider(ctx context.Context, entry providersvc.Provider) error
	DefaultProvider() string
}

type mcpAccess interface {
	MCPServerStatuses() []kernel.McpServerStatus
	ReconnectMCPServer(ctx context.Context, name string) error
	AuthorizeMCPServer(ctx context.Context, name string) error
	MCPTools(ctx context.Context, server string) ([]kernel.McpToolInfo, error)
	ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error)
	GetMCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error)
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
	HasCodebaseIndex() bool
	SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error)
	CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error)
	StartCodebaseReindex(ctx context.Context, root string) error
}

type maintenanceAccess interface {
	GenerateTitle(ctx context.Context, firstMessage string) (string, error)
}
