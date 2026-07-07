package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

func buildToolEnvironment(ctx context.Context, cfg Config, ecfg kernel.Config, approvalPolicy approval.Policy, mcpEnv mcpEnvironment, codebaseIdx codebaseindex.Index) (toolset.Built, error) {
	built, err := toolset.Build(ctx, toolset.BuildConfig{
		Workdir:         cfg.Engine.Workdir,
		SkillsGlobalDir: cfg.Engine.SkillsGlobalDir,
		Online:          toolset.OnlineConfig(cfg.Online),
		LSPServers:      cfg.LSPServers,
		MCPServers:      mcpEnv.configs,
		A2AAgents:       cfg.A2AAgents,
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
