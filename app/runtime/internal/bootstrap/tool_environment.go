package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/mcpconnection"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	planapp "github.com/Tangerg/lynx/app/runtime/internal/application/plans"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
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
	lifetime context.Context,
	cfg Config,
	approvalPolicy *approvals.RuntimePolicy,
	mcpConnectionSettings mcpEnvironment,
	agentMemorySearcher *agentmemory.Searcher,
	scheduleCoordinator *schedules.Coordinator,
	goalReader *goals.Reader,
	goalReporter *goals.OutcomeReporter,
	planCoordinator *planapp.Coordinator,
	skillStore *skillauthoring.Store,
	skillProposals builtin.SkillProposalSubmitter,
) (toolEnvironment, error) {
	mcpPool, mcpTools, err := mcpconnection.Open(
		ctx,
		lifetime,
		mcpConnectionSettings.servers,
		cfg.MCPOAuthSessions,
	)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: open MCP connections: %w", err)
	}
	environment := toolEnvironment{
		mcp:     mcpPool,
		closers: []ShutdownResource{mcpPool},
	}
	buildConfig := toolset.BuildConfig{
		Lifetime:        lifetime,
		DefaultCWD:      cfg.DefaultWorkspacePath,
		UserHome:        cfg.UserHome,
		SkillsUserDir:   cfg.SkillsUserDir,
		Online:          cfg.Online,
		LSPServers:      cfg.LSPServers,
		MCPTools:        mcpTools,
		A2AAgents:       cfg.A2AAgents,
		Plan:            planCoordinator,
		Interrupt:       agentexec.RequireToolInput,
		MCPToolDisabled: mcpConnectionSettings.policy.ToolDisabled,
		// The authoring store records Skill loads for idle-Skill archival; a
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
		buildConfig.PlanMode = approvalPolicy
	}
	if cfg.ScheduleStore != nil {
		buildConfig.Schedules = scheduleCoordinator
	}
	// Set the read-back store only when concretely present, so a nil store never
	// reaches the tool builder as a non-nil interface holding a nil pointer.
	if cfg.ToolResultStore != nil {
		buildConfig.ToolResults = cfg.ToolResultStore
	}
	// Goal reads, outcome reports, and the active gate come from separate
	// application boundaries. create_goal is injected later when the Driver exists.
	if goalReader != nil {
		buildConfig.GoalReader = goalReader
	}
	if goalReporter != nil {
		buildConfig.GoalReporter = goalReporter
	}
	// search_memory searches the agent's curated project memory. Set only when a
	// concrete searcher exists, so a nil *Searcher never reaches the tool builder
	// as a non-nil interface.
	if agentMemorySearcher != nil {
		buildConfig.AgentMemorySearch = agentMemorySearcher
	}
	// search_conversations recalls past conversation transcripts (the durable Item
	// history). Set only when the concrete store is present, for the same
	// nil-interface reason.
	if cfg.TranscriptStore != nil {
		buildConfig.ConversationSearch = cfg.TranscriptStore
	}
	builtToolset, err := toolset.Build(ctx, buildConfig)
	if err != nil {
		return environment, fmt.Errorf("runtime: build tools: %w", err)
	}
	mcpPool.SetToolSink(builtToolset.Resolver.SetMCPTools)
	environment.tools = builtToolset
	environment.closers = append(environment.closers, shutdownClosers(builtToolset.Closers)...)
	return environment, nil
}
