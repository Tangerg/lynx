package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

func buildToolEnvironment(ctx context.Context, cfg Config, ecfg agentexec.Config, approvalPolicy approval.Policy, mcpEnv mcpEnvironment, codebaseIdx toolset.CodebaseIndex, skillStore *skillauthoring.Store) (toolset.Built, error) {
	bc := toolset.BuildConfig{
		Workdir:         cfg.Engine.Workdir,
		SkillsGlobalDir: cfg.SkillsGlobalDir,
		Online:          toolset.OnlineConfig(cfg.Online),
		LSPServers:      codeintelServerSpecs(cfg.LSPServers),
		MCPServers:      mcpEnv.configs,
		A2AAgents:       toolsetA2AAgentConfigs(cfg.A2AAgents),
		Todos:           ecfg.Todos,
		Approval:        approvalPolicy,
		Interrupt:       hitl.Interrupt[interrupts.Resolution],
		Schedules:       cfg.ScheduleRegistry,
		MCPToolDisabled: mcpEnv.toolDisabled,
		CodebaseIndex:   codebaseIdx,
		// propose_skill writes to the global skills dir; an empty dir yields a
		// disabled store (Enabled() false), which omits the tool.
		SkillAuthoring: skillStore,
	}
	// Set the read-back store only when concretely present, so a nil store never
	// reaches the tool builder as a non-nil interface holding a nil pointer.
	if cfg.ToolResultStore != nil {
		bc.ToolResults = cfg.ToolResultStore
	}
	built, err := toolset.Build(ctx, bc)
	if err != nil {
		return toolset.Built{}, fmt.Errorf("runtime: build tools: %w", err)
	}
	return built, nil
}

func toolsetA2AAgentConfigs(in []A2AAgentConfig) []toolset.A2AAgentConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]toolset.A2AAgentConfig, len(in))
	for i, agent := range in {
		out[i] = toolset.A2AAgentConfig{
			Name:    agent.Name,
			CardURL: agent.CardURL,
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
