package bootstrap

import (
	"io"
	"path/filepath"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/pricing"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// RuntimeConfig assembles the runtime Config from already-opened
// process adapters.
func RuntimeConfig(cfg config.Config, stores *persistence.Bundle, client *chat.Client, providers providersvc.Registry, hooks HookResolver) Config {
	return Config{
		Resources: []io.Closer{stores},
		Engine: agentexec.Config{
			ChatClient:      client,
			Pricing:         pricing.Catalog(),
			SkillsGlobalDir: filepath.Join(stores.Home, "skills"),
			HistoryStore:    stores.ChatHistory,
			Knowledge:       stores.Memory,
			ParkStore:       stores.Park,
		},
		UtilityRoleStore:       stores.UtilityRole,
		Online:                 OnlineConfig(cfg.Online),
		MCPRegistry:            stores.MCPServers,
		A2AAgents:              runtimeA2AAgents(cfg.A2AAgents),
		LSPServers:             runtimeLSPServers(cfg.LSPServers),
		SessionStore:           stores.Session,
		RunStore:               stores.Runs,
		ProcessStore:           stores.Process,
		WorkspaceMutationStore: stores.WorkspaceMuts,
		InterruptStore:         stores.Interrupt,
		TranscriptStore:        stores.Transcript,
		ProviderRegistry:       providers,
		TodoStore:              stores.Todos,
		Provider:               cfg.Provider,
		Model:                  cfg.Model,
		HooksResolver:          hooks,
		HookTrustStore:         stores.Trust,
		RecipesGlobalDir:       filepath.Join(stores.Home, "recipes"),
		CheckpointDir:          filepath.Join(stores.Home, "checkpoints"),
		ScheduleRegistry:       stores.Schedules,
		EmbeddingRoleStore:     stores.EmbeddingRole,
		CodebaseStore:          stores.Codebase,
		Transactor:             Transactor(stores.Tx),
		ApprovalMode:           approval.ModeBalanced,
		ApprovalRuleStore:      stores.ApprovalRules,
	}
}
