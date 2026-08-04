package bootstrap

import (
	"path/filepath"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/pricing"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/chatclient"
)

// ComposeConfig translates process settings and already-opened adapters into
// the construction input consumed by [NewAssembly].
func ComposeConfig(cfg config.Settings, stores *persistence.Bundle, client *chatclient.Client, providers providersvc.Registry, hooks HookResolver, buildID string) Config {
	return Config{
		Resources:     []ShutdownResource{stores},
		SkillsUserDir: filepath.Join(stores.DataDirectory, "skills"),
		Engine: agentexec.Config{
			BuildID:      buildID,
			ChatClient:   client,
			Pricing:      pricing.Catalog(),
			HistoryStore: stores.ChatHistory,
			AgentMemory:  stores.AgentMemory,
		},
		AgentMemoryStore:       stores.AgentMemory,
		IdempotencyStore:       stores.Idempotency,
		UtilityRoleStore:       stores.UtilityRole,
		Online:                 toolset.OnlineConfig(cfg.Online),
		MCPRegistry:            stores.MCPServers,
		MCPOAuthSessions:       stores.MCPServers,
		A2AAgents:              toolsetA2AAgents(cfg.A2AAgents),
		LSPServers:             codeintelServers(cfg.LSPServers),
		SandboxShell:           cfg.SandboxShell,
		SandboxReadOnlyPaths:   cfg.SandboxReadOnlyPaths,
		SandboxDir:             filepath.Join(stores.DataDirectory, "sandbox"),
		SessionStore:           stores.Session,
		RunStore:               stores.Runs,
		ExecutorCheckpoints:    stores.ExecutorCheckpoints,
		WorkspaceMutationStore: stores.WorkspaceMuts,
		InterruptStore:         stores.Interrupt,
		TranscriptStore:        stores.Transcript,
		FeedbackStore:          stores.Feedback,
		ProviderRegistry:       providers,
		PlanStore:              stores.Plan,
		PermissionModeStore:    stores.PermissionModes,
		GoalStore:              stores.Goals,
		KnowledgeStore:         stores.Memory,
		Provider:               cfg.Provider,
		Model:                  cfg.Model,
		HooksResolver:          hooks,
		HookTrustStore:         stores.Trust,
		RecipesGlobalDir:       filepath.Join(stores.DataDirectory, "recipes"),
		CheckpointDir:          filepath.Join(stores.DataDirectory, "checkpoints"),
		ScheduleStore:          stores.Schedules,
		EmbeddingRoleStore:     stores.EmbeddingRole,
		CodebaseStore:          stores.Codebase,
		ToolResultStore:        stores.ToolResults,
		ToolResultThreshold:    cfg.ToolResultOffloadThreshold,
		Transactor:             Transactor(stores.Tx),
		ApprovalMode:           approval.ModeBalanced,
		ApprovalRuleStore:      stores.ApprovalRules,
	}
}

func toolsetA2AAgents(in []config.A2AAgent) []toolset.A2AAgentConfig {
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

func codeintelServers(in []config.LSPServer) []codeintel.ServerSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]codeintel.ServerSpec, len(in))
	for i, server := range in {
		out[i] = codeintel.ServerSpec{
			Name:        server.Name,
			Command:     server.Command,
			Args:        slices.Clone(server.Args),
			LanguageID:  server.LanguageID,
			Extensions:  slices.Clone(server.Extensions),
			RootMarkers: slices.Clone(server.RootMarkers),
		}
	}
	return out
}
