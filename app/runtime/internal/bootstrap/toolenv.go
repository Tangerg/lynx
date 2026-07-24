package bootstrap

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/mcpconnection"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

// toolEnvironment groups the tool resolver with the separately-owned MCP
// connection adapter. Bootstrap is the composition root that joins them; the
// generic toolset does not expose application integration ports.
type toolEnvironment struct {
	tools   toolset.Built
	mcp     *mcpconnection.Connections
	closers []func() error
}

func buildToolEnvironment(
	ctx context.Context,
	cfg Config,
	ecfg agentexec.Config,
	approvalPolicy *approval.RuntimePolicy,
	mcpEnv mcpEnvironment,
	codebaseIdx toolset.CodebaseIndex,
	memorySearcher *agentmemory.Searcher,
	scheduleCoord *schedules.Coordinator,
	goalState *goals.State,
	skillStore *skillauthoring.Store,
) (toolEnvironment, error) {
	mcpConnections, mcpTools, err := mcpconnection.Open(ctx, mcpEnv.servers)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: open MCP connections: %w", err)
	}
	mcpOpen := true
	defer func() {
		if mcpOpen {
			_ = mcpConnections.Close()
		}
	}()
	bc := toolset.BuildConfig{
		Workdir:         cfg.Engine.Workdir,
		SkillsGlobalDir: cfg.SkillsGlobalDir,
		Online:          toolset.OnlineConfig(cfg.Online),
		LSPServers:      codeintelServerSpecs(cfg.LSPServers),
		MCPTools:        mcpTools,
		A2AAgents:       toolsetA2AAgentConfigs(cfg.A2AAgents),
		Todos:           cfg.TodoStore,
		Approval:        approvalPolicy,
		Interrupt:       suspension.Interrupt,
		MCPToolDisabled: mcpEnv.policy.ToolDisabled,
		CodebaseIndex:   codebaseIdx,
		// propose_skill writes to the global skills dir; an empty dir yields a
		// disabled store (Enabled() false), which omits the tool.
		SkillAuthoring: skillStore,
		// The same store records skill loads for the idle-lifecycle curator; a
		// disabled store no-ops RecordUse.
		SkillUsage: skillStore,
		// Opt-in per-command OS isolation for the shell tools (off by default).
		SandboxShell:         cfg.SandboxShell,
		SandboxReadOnlyPaths: cfg.SandboxReadOnlyPaths,
	}
	if cfg.ScheduleStore != nil {
		bc.Schedules = scheduleCoord
	}
	// Set the read-back store only when concretely present, so a nil store never
	// reaches the tool builder as a non-nil interface holding a nil pointer.
	if cfg.ToolResultStore != nil {
		bc.ToolResults = cfg.ToolResultStore
	}
	// update_goal + its active-gate come from the application state boundary. Set only
	// when present, for the same nil-interface reason.
	if goalState != nil {
		bc.Goals = goalState
	}
	// memory_search searches the agent's curated project memory. Set only when a
	// concrete searcher exists, so a nil *Searcher never reaches the tool builder
	// as a non-nil interface.
	if memorySearcher != nil {
		bc.MemorySearch = memorySearcher
	}
	// session_search recalls past conversation transcripts (the durable Item
	// history). Set only when the concrete store is present, for the same
	// nil-interface reason.
	if cfg.TranscriptStore != nil {
		bc.SessionSearch = cfg.TranscriptStore
	}
	built, err := toolset.Build(ctx, bc)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: build tools: %w", err)
	}
	mcpConnections.SetToolSink(built.Resolver.SetMCPTools)
	mcpOpen = false
	return toolEnvironment{
		tools:   built,
		mcp:     mcpConnections,
		closers: append(built.Closers, mcpConnections.Close),
	}, nil
}

func toolsetA2AAgentConfigs(in []A2AAgentConfig) []toolset.A2AAgentConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]toolset.A2AAgentConfig, len(in))
	for i, agent := range in {
		out[i] = toolset.A2AAgentConfig{
			Name:              agent.Name,
			CardURL:           agent.CardURL,
			AllowedRPCOrigins: slices.Clone(agent.AllowedRPCOrigins),
		}
	}
	return out
}

func codeintelServerSpecs(in []LSPServerConfig) []codeintel.ServerSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]codeintel.ServerSpec, len(in))
	for i, server := range in {
		out[i] = codeintel.ServerSpec{
			Name:        server.Name,
			Command:     server.Command,
			Args:        server.Args,
			LanguageID:  server.LanguageID,
			Extensions:  server.Extensions,
			RootMarkers: server.RootMarkers,
		}
	}
	return out
}
