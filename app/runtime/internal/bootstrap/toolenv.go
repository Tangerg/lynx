package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/mcpconnection"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

// toolEnvironment groups the tool resolver with the separately-owned MCP
// connection adapter. Bootstrap is the composition root that joins them; the
// generic toolset does not expose application integration ports.
type toolEnvironment struct {
	tools   toolset.Built
	mcp     *mcpconnection.Pool
	closers []ShutdownResource
}

func buildToolEnvironment(
	ctx context.Context,
	cfg Config,
	ecfg agentexec.Config,
	approvalPolicy *approval.RuntimePolicy,
	mcpEnv mcpEnvironment,
	memorySearcher *agentmemory.Searcher,
	scheduleCoord *schedules.Coordinator,
	goalState *goals.State,
	skillStore *skillauthoring.Store,
	skillProposals skill.ProposalSubmitter,
) (toolEnvironment, error) {
	defaultModel, err := modelref.New(cfg.Provider, cfg.Model)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: default tool model: %w", err)
	}
	mcpPool, mcpTools, err := mcpconnection.Open(ctx, mcpEnv.servers, cfg.MCPOAuthSessions)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: open MCP connections: %w", err)
	}
	environment := toolEnvironment{
		mcp:     mcpPool,
		closers: []ShutdownResource{mcpPool},
	}
	bc := toolset.BuildConfig{
		Workdir:         ecfg.Workdir,
		UserHome:        ecfg.UserHome,
		DefaultModel:    defaultModel,
		SkillsUserDir:   cfg.SkillsUserDir,
		Online:          cfg.Online,
		LSPServers:      cfg.LSPServers,
		MCPTools:        mcpTools,
		A2AAgents:       cfg.A2AAgents,
		Plan:            cfg.PlanStore,
		Interrupt:       suspension.Interrupt,
		MCPToolDisabled: mcpEnv.policy.ToolDisabled,
		// The authoring store records skill loads for the idle-lifecycle curator; a
		// disabled store no-ops RecordUse.
		SkillUsage:     skillStore,
		SkillProposals: skillProposals,
		// Opt-in per-command OS isolation for the shell tools (off by default).
		SandboxShell:         cfg.SandboxShell,
		SandboxReadOnlyPaths: cfg.SandboxReadOnlyPaths,
	}
	// Plan mode is usable only when both durable pieces exist: the permission
	// overlay and the canonical Plan it eventually presents for approval. Keep
	// the mode tools absent in partial test configurations instead of exposing a
	// capability that can enter but cannot exit.
	if cfg.PermissionModeStore != nil && cfg.PlanStore != nil {
		bc.PlanMode = approvalPolicy
	}
	if cfg.ScheduleStore != nil {
		bc.Schedules = scheduleCoord
	}
	// Set the read-back store only when concretely present, so a nil store never
	// reaches the tool builder as a non-nil interface holding a nil pointer.
	if cfg.ToolResultStore != nil {
		bc.ToolResults = cfg.ToolResultStore
	}
	// Goal reads/outcome reports and the active gate come from the application
	// state boundary. create_goal is injected later when the Driver exists. Set
	// only when present, for the same nil-interface reason.
	if goalState != nil {
		bc.Goals = goalState
	}
	// search_memory searches the agent's curated project memory. Set only when a
	// concrete searcher exists, so a nil *Searcher never reaches the tool builder
	// as a non-nil interface.
	if memorySearcher != nil {
		bc.MemorySearch = memorySearcher
	}
	// search_conversations recalls past conversation transcripts (the durable Item
	// history). Set only when the concrete store is present, for the same
	// nil-interface reason.
	if cfg.TranscriptStore != nil {
		bc.SessionSearch = cfg.TranscriptStore
	}
	built, err := toolset.Build(ctx, bc)
	if err != nil {
		return environment, fmt.Errorf("runtime: build tools: %w", err)
	}
	mcpPool.SetToolSink(built.Resolver.SetMCPTools)
	environment.tools = built
	environment.closers = append(environment.closers, shutdownClosers(built.Closers)...)
	return environment, nil
}
