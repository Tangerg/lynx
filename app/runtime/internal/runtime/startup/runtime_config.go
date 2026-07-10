package startup

import (
	"io"
	"path/filepath"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/pricing"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// RuntimeConfig assembles the runtime facade config from already-opened
// process adapters.
func RuntimeConfig(cfg config.Config, stores *persistence.Bundle, client *chat.Client, providers providersvc.Registry, hooks lyraruntime.HookResolver) lyraruntime.Config {
	return lyraruntime.Config{
		Resources: []io.Closer{stores},
		Engine: kernel.Config{
			ChatClient:      client,
			Pricing:         pricing.Catalog(),
			SkillsGlobalDir: filepath.Join(stores.Home, "skills"),
			HistoryStore:    stores.ChatHistory,
			Knowledge:       stores.Memory,
			ProcessStore:    stores.Process,
			ParkStore:       stores.Park,
		},
		UtilityRoleStore:   stores.UtilityRole,
		Online:             lyraruntime.OnlineConfig(cfg.Online),
		MCPRegistry:        stores.MCPServers,
		A2AAgents:          runtimeA2AAgents(cfg.A2AAgents),
		LSPServers:         runtimeLSPServers(cfg.LSPServers),
		SessionStore:       stores.Session,
		InterruptStore:     stores.Interrupt,
		TranscriptStore:    stores.Transcript,
		ProviderRegistry:   providers,
		TodoStore:          stores.Todos,
		Provider:           cfg.Provider,
		Model:              cfg.Model,
		HooksResolver:      hooks,
		HookTrustStore:     stores.Trust,
		RecipesGlobalDir:   filepath.Join(stores.Home, "recipes"),
		ScheduleRegistry:   stores.Schedules,
		EmbeddingRoleStore: stores.EmbeddingRole,
		CodebaseStore:      stores.Codebase,
		Transactor:         lyraruntime.Transactor(stores.Tx),
		ApprovalMode:       approval.ModeBalanced,
		ApprovalRuleStore:  stores.ApprovalRules,
	}
}
