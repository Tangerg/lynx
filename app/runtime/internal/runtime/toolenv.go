package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

func buildToolEnvironment(ctx context.Context, cfg Config, ecfg kernel.Config, approvalPolicy approval.Policy, mcpEnv mcpEnvironment, codebaseIdx toolset.CodebaseIndex) (toolset.Built, error) {
	built, err := toolset.Build(ctx, toolset.BuildConfig{
		Workdir:         cfg.Engine.Workdir,
		SkillsGlobalDir: cfg.Engine.SkillsGlobalDir,
		Online:          toolset.OnlineConfig(cfg.Online),
		LSPServers:      codeintelServerSpecs(cfg.LSPServers),
		MCPServers:      mcpEnv.configs,
		A2AAgents:       toolsetA2AAgentConfigs(cfg.A2AAgents),
		Todos:           ecfg.Todos,
		Approval:        approvalPolicy,
		Interruption:    kernel.Interrupt[interrupts.Resolution],
		Schedules:       cfg.ScheduleRegistry,
		MCPDisabled:     mcpEnv.disabled,
		CodebaseIndex:   codebaseIdx,
	})
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
