package runtime

import (
	"io"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

type runtimeFacadeDeps struct {
	cfg       Config
	engine    *kernel.Engine
	turns     turn.Dispatcher
	tools     toolsvc.Registry
	approval  approval.Policy
	messages  messageEnvironment
	resolver  chatClientResolver
	mcp       mcpEnvironment
	utility   utilityEnvironment
	embedding embeddingEnvironment
}

func newRuntimeFacade(d runtimeFacadeDeps) *Runtime {
	schedules := d.cfg.ScheduleRegistry
	if schedules == nil {
		schedules = disabledScheduleRegistry{}
	}
	return &Runtime{
		turns:              d.turns,
		closer:             d.engine,
		resources:          append([]io.Closer(nil), d.cfg.Resources...),
		skillCatalog:       d.engine,
		tools:              d.tools,
		memory:             d.cfg.Engine.Knowledge,
		approval:           d.approval,
		history:            d.messages.conversation,
		sessions:           d.cfg.SessionStore,
		interrupts:         d.cfg.InterruptStore,
		transcript:         d.cfg.TranscriptStore,
		providers:          d.cfg.ProviderRegistry,
		mcpRegistry:        d.cfg.MCPRegistry,
		mcpLiveStatus:      d.engine,
		mcpLiveTools:       d.engine,
		mcpLiveConnections: d.engine,
		mcpLiveRegistry:    d.engine,
		mcpPolicy:          d.mcp.policy,
		defaultProvider:    d.cfg.Provider,
		defaultModel:       d.cfg.Model,
		titles:             maintenance.NewTitler(d.utility.resolve),
		utility:            d.utility.cell,
		utilityClients:     d.resolver,
		utilStore:          d.cfg.UtilityRoleStore,
		hookInspection:     d.cfg.HooksResolver,
		hookTrust:          d.cfg.HookTrustStore,
		recipesGlobalDir:   d.cfg.RecipesGlobalDir,
		schedules:          schedules,
		scheduleWorker:     d.cfg.ScheduleRegistry,
		embeddingCell:      d.embedding.cell,
		embeddings:         d.embedding.resolver,
		embeddingStore:     d.cfg.EmbeddingRoleStore,
		codebase:           d.embedding.index,
		transactor:         d.cfg.Transactor,
	}
}
