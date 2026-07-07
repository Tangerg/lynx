package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

func buildToolEnvironment(ctx context.Context, cfg Config, ecfg kernel.Config, approvalPolicy approval.Policy, mcpEnv mcpEnvironment, codebaseIdx codebaseindex.Index) (toolset.Built, error) {
	built, err := toolset.Build(ctx, toolset.BuildConfig{
		Workdir:         cfg.Engine.Workdir,
		SkillsGlobalDir: cfg.Engine.SkillsGlobalDir,
		Online:          toolset.OnlineConfig(cfg.Online),
		LSPServers:      codeintelServerSpecs(cfg.LSPServers),
		MCPServers:      mcpEnv.configs,
		A2AAgents:       a2aClientConfigs(cfg.A2AAgents),
		Todos:           ecfg.Todos,
		Approval:        approvalPolicy,
		MCPDisabled:     mcpEnv.disabled,
		CodebaseIndex:   codebaseIdx,
	})
	if err != nil {
		return toolset.Built{}, fmt.Errorf("runtime: build tools: %w", err)
	}
	return built, nil
}

func a2aClientConfigs(in []A2AAgentConfig) []a2a.ClientConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]a2a.ClientConfig, len(in))
	for i, agent := range in {
		out[i] = a2a.ClientConfig{
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
