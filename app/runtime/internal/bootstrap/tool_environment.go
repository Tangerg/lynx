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
	"github.com/Tangerg/lynx/app/runtime/internal/infra/teardown"
)

// toolEnvironment groups the tool resolver with the separately-owned MCP
// connection adapter. Bootstrap is the composition root that joins them; the
// generic toolset does not expose application integration ports.
type toolEnvironment struct {
	tools   toolset.Built
	mcp     *mcpconnection.Pool
	closers []*teardown.Step
}

// toolEnvironmentDependencies is the complete construction contract for the
// process-owned tool runtime. Keeping the contract as one value lets Assembly
// transfer every acquisition to hostLifetime even when construction fails.
type toolEnvironmentDependencies struct {
	lifetime            context.Context
	config              Config
	approvalPolicy      *approvals.RuntimePolicy
	mcp                 mcpEnvironment
	agentMemorySearcher *agentmemory.Searcher
	schedules           *schedules.Coordinator
	goalReader          *goals.Reader
	goalReporter        *goals.OutcomeReporter
	plan                *planapp.Coordinator
	skillStore          *skillauthoring.Store
	skillProposals      builtin.SkillProposalSubmitter
}

type toolEnvironmentBuilder func(context.Context, toolEnvironmentDependencies) (toolEnvironment, error)

func buildToolEnvironment(ctx context.Context, deps toolEnvironmentDependencies) (toolEnvironment, error) {
	cfg := deps.config
	mcpPool, mcpTools, err := mcpconnection.Open(
		ctx,
		deps.lifetime,
		deps.mcp.servers,
		cfg.MCPOAuthSessions,
	)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: open MCP connections: %w", err)
	}
	environment := toolEnvironment{
		mcp: mcpPool,
		// The SDK consumes each ClientSession transport closer even when Close
		// returns a diagnostic. Step owns the caller deadline; the action itself
		// must outlive that deadline and return only when the pool generation has
		// actually settled, so a timed-out Host retains and later joins it.
		closers: []*teardown.Step{teardown.Terminal(func(ctx context.Context) error {
			return mcpPool.Shutdown(context.WithoutCancel(ctx))
		})},
	}
	buildConfig := toolset.BuildConfig{
		Lifetime:        deps.lifetime,
		DefaultCWD:      cfg.DefaultWorkspacePath,
		UserHome:        cfg.UserHome,
		SkillsUserDir:   cfg.SkillsUserDir,
		Online:          cfg.Online,
		LSPServers:      cfg.LSPServers,
		MCPTools:        mcpTools,
		A2AAgents:       cfg.A2AAgents,
		Plan:            deps.plan,
		Interrupt:       agentexec.RequireToolInput,
		MCPToolDisabled: deps.mcp.policy.ToolDisabled,
		// The authoring store records Skill loads for idle-Skill archival; a
		// disabled store no-ops RecordUse.
		SkillUsage:     deps.skillStore,
		SkillProposals: deps.skillProposals,
		// Opt-in per-command OS isolation for the shell tools (off by default).
		SandboxShell:         cfg.SandboxShell,
		SandboxReadOnlyPaths: cfg.SandboxReadOnlyPaths,
	}
	// Plan mode is usable only when both durable pieces exist: the permission
	// overlay and the canonical Plan it eventually presents for approval. Keep
	// the mode tools absent in partial test configurations instead of exposing a
	// capability that can enter but cannot exit.
	if cfg.PermissionModeStore != nil && cfg.PlanStore != nil {
		buildConfig.PlanMode = deps.approvalPolicy
	}
	if cfg.ScheduleStore != nil {
		buildConfig.Schedules = deps.schedules
	}
	// Set the read-back store only when concretely present, so a nil store never
	// reaches the tool builder as a non-nil interface holding a nil pointer.
	if cfg.ToolResultStore != nil {
		buildConfig.ToolResults = cfg.ToolResultStore
	}
	// Goal reads, outcome reports, and the active gate come from separate
	// application boundaries. create_goal is injected later when the Driver exists.
	if deps.goalReader != nil {
		buildConfig.GoalReader = deps.goalReader
	}
	if deps.goalReporter != nil {
		buildConfig.GoalReporter = deps.goalReporter
	}
	// search_memory searches the agent's curated project memory. Set only when a
	// concrete searcher exists, so a nil *Searcher never reaches the tool builder
	// as a non-nil interface.
	if deps.agentMemorySearcher != nil {
		buildConfig.AgentMemorySearch = deps.agentMemorySearcher
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
	environment.closers = append(environment.closers, terminalClosers(builtToolset.Closers)...)
	return environment, nil
}
