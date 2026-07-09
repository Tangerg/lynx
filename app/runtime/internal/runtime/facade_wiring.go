package runtime

import (
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
		turns:                     d.turns,
		closer:                    d.engine,
		skillCatalog:              d.engine,
		a2aChats:                  d.engine,
		toolCatalog:               d.tools,
		toolInvocations:           d.tools,
		memoryList:                d.cfg.Engine.Knowledge,
		memoryRead:                d.cfg.Engine.Knowledge,
		memoryWrite:               d.cfg.Engine.Knowledge,
		approvalModeRead:          d.approval,
		approvalModeMutation:      d.approval,
		approvalRuleList:          d.approval,
		approvalRuleDeletion:      d.approval,
		history:                   d.messages.conversation,
		sessionList:               d.cfg.SessionStore,
		sessionRead:               d.cfg.SessionStore,
		sessionCreation:           d.cfg.SessionStore,
		sessionPatch:              d.cfg.SessionStore,
		sessionModel:              d.cfg.SessionStore,
		sessionLifecycle:          d.cfg.SessionStore,
		sessionRunSegment:         d.cfg.SessionStore,
		interruptList:             d.cfg.InterruptStore,
		interruptLifecycle:        d.cfg.InterruptStore,
		interruptRunSegment:       d.cfg.InterruptStore,
		transcriptContent:         d.cfg.TranscriptStore,
		transcriptRuns:            d.cfg.TranscriptStore,
		transcriptLifecycle:       d.cfg.TranscriptStore,
		transcriptRunSegment:      d.cfg.TranscriptStore,
		providerRegistryList:      d.cfg.ProviderRegistry,
		providerRegistryRead:      d.cfg.ProviderRegistry,
		providerRegistryConfigure: d.cfg.ProviderRegistry,
		mcpRegistryList:           d.cfg.MCPRegistry,
		mcpRegistryRead:           d.cfg.MCPRegistry,
		mcpRegistryConfigure:      d.cfg.MCPRegistry,
		mcpRegistryRemove:         d.cfg.MCPRegistry,
		mcpRegistryEnable:         d.cfg.MCPRegistry,
		mcpLiveStatus:             d.engine,
		mcpLiveTools:              d.engine,
		mcpLiveConnections:        d.engine,
		mcpLiveRegistry:           d.engine,
		mcpGating:                 d.mcp.gate,
		defaultProvider:           d.cfg.Provider,
		defaultModel:              d.cfg.Model,
		titles:                    maintenance.NewTitler(d.utility.resolve),
		utility:                   d.utility.cell,
		utilityClients:            d.resolver,
		utilStore:                 d.cfg.UtilityRoleStore,
		hookInspection:            d.cfg.HooksResolver,
		hookTrust:                 d.cfg.HookTrustStore,
		recipesGlobalDir:          d.cfg.RecipesGlobalDir,
		scheduleList:              schedules,
		scheduleRead:              schedules,
		scheduleCreation:          schedules,
		scheduleUpdates:           schedules,
		scheduleDeletion:          schedules,
		scheduleRuns:              schedules,
		scheduleWorker:            d.cfg.ScheduleRegistry,
		embeddingCell:             d.embedding.cell,
		embeddings:                d.embedding.resolver,
		embeddingStore:            d.cfg.EmbeddingRoleStore,
		codebaseAvailability:      d.embedding.index,
		codebaseSearch:            d.embedding.index,
		codebaseStatus:            d.embedding.index,
		codebaseReindex:           d.embedding.index,
		transactor:                d.cfg.Transactor,
	}
}
