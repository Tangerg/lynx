// Package dispatch is the JSON-RPC ↔ Runtime bridge. It owns the
// mapping from JSON-RPC method names (API.md §7) to typed Runtime
// calls + the inverse encoding of results / errors / events into
// JSON-RPC envelopes.
//
// Single responsibility:
//
//	Decode  inbound transport.Message → resolve method name → unmarshal params
//	        → call Runtime method → marshal result OR map error to {code, type}.
//	Encode  Runtime RunEvent stream  → notifications.run.event envelopes.
//
// The dispatcher is stateless: request metadata travels inside params._meta and
// is exposed on context for handlers that need client-scoped capabilities. The
// /v2/info and /v2/health/{live,ready} sidecars don't go through this
// dispatcher (flat REST handled by delivery/transport/http).
package dispatch

// JSON-RPC method names — single source for everywhere that needs to
// branch on them. Untyped string constants match the JSON wire shape
// one-to-one and let the HTTP path-cross-check stay a plain compare.
// Names are dotted <domain>.<verb> (API.md §2.3); HTTP keeps the dots.
const (
	// Lifecycle (API.md §7.1).
	MethodDiscover = "runtime.discover"

	// Sessions (API.md §7.2).
	MethodSessionsList     = "sessions.list"
	MethodSessionsGet      = "sessions.get"
	MethodSessionsCreate   = "sessions.create"
	MethodSessionsUpdate   = "sessions.update"
	MethodSessionsDelete   = "sessions.delete"
	MethodSessionsFork     = "sessions.fork"
	MethodSessionsRollback = "sessions.rollback"
	MethodSessionsExport   = "sessions.export"
	MethodSessionsImport   = "sessions.import"

	// Runs (API.md §7.3). HITL is the R model: runs.resume answers
	// open interrupts via a continuation run.
	MethodRunsStart              = "runs.start"
	MethodRunsResume             = "runs.resume"
	MethodRunsSubscribe          = "runs.subscribe"
	MethodRunsCancel             = "runs.cancel"
	MethodRunsSteer              = "runs.steer"
	MethodRunsList               = "runs.list"
	MethodRunsListOpenInterrupts = "runs.listOpenInterrupts"

	// Items (API.md §7.4). History = the completed Item sequence.
	MethodItemsList = "items.list"

	// Workspace (API.md §7.5).
	MethodWorkspaceListFileChanges = "workspace.listFileChanges"
	MethodWorkspaceGetDiff         = "workspace.getDiff"
	MethodWorkspaceGetFileHead     = "workspace.getFileHead"
	MethodWorkspaceGrep            = "workspace.grep"
	MethodWorkspaceListFiles       = "workspace.listFiles"
	MethodWorkspaceReadFile        = "workspace.readFile"
	MethodWorkspaceListProjects    = "workspace.listProjects"
	MethodWorkspaceSubscribe       = "workspace.subscribe"

	// Discovery and integration domains (API.md §7.5).
	MethodSkillsDiscoveredList = "skills.discovered.list"
	MethodSkillsLibraryList    = "skills.library.list"
	MethodSkillsLibraryArchive = "skills.library.archive"
	MethodSkillsLibraryRestore = "skills.library.restore"
	MethodRecipesList          = "recipes.list"
	MethodAgentDocsList        = "agentDocs.list"
	MethodMCPServersList       = "mcp.servers.list"
	MethodMCPServersReconnect  = "mcp.servers.reconnect"
	MethodMCPServersAuthorize  = "mcp.servers.authorize"
	MethodMCPToolsList         = "mcp.tools.list"
	MethodMCPConfigsList       = "mcp.configs.list"
	MethodMCPConfigsConfigure  = "mcp.configs.configure"
	MethodMCPConfigsRemove     = "mcp.configs.remove"
	MethodMCPConfigsSetEnabled = "mcp.configs.setEnabled"
	MethodMCPConfigsTest       = "mcp.configs.test"
	MethodHooksList            = "hooks.list"
	MethodHooksSetTrust        = "hooks.setTrust"

	// Approval (API.md §C.3) — runtime-global tool-permission stance + the
	// persistent fine-grained "remember this decision" rules.
	MethodApprovalGetMode    = "approval.getMode"
	MethodApprovalSetMode    = "approval.setMode"
	MethodApprovalListRules  = "approval.listRules"
	MethodApprovalForgetRule = "approval.forgetRule"

	// Schedules (API.md §7.9) — cron-triggered headless runs of a saved prompt.
	MethodSchedulesList   = "schedules.list"
	MethodSchedulesCreate = "schedules.create"
	MethodSchedulesUpdate = "schedules.update"
	MethodSchedulesDelete = "schedules.delete"
	MethodSchedulesRunNow = "schedules.runNow"

	// Goals (API.md §7.14) — Goal mode, the autonomous execution loop.
	MethodGoalsStart  = "goals.start"
	MethodGoalsGet    = "goals.get"
	MethodGoalsStop   = "goals.stop"
	MethodGoalsResume = "goals.resume"

	// Codebase (API.md §7.10) — the @codebase semantic index.
	MethodCodebaseSearch  = "codebase.search"
	MethodCodebaseStatus  = "codebase.status"
	MethodCodebaseReindex = "codebase.reindex"

	// Providers / Models / Tools (API.md §7.6).
	MethodProvidersList          = "providers.list"
	MethodProvidersConfigure     = "providers.configure"
	MethodProvidersTest          = "providers.test"
	MethodModelsList             = "models.list"
	MethodModelsGetUtilityRole   = "models.getUtilityRole"
	MethodModelsSetUtilityRole   = "models.setUtilityRole"
	MethodModelsGetEmbeddingRole = "models.getEmbeddingRole"
	MethodModelsSetEmbeddingRole = "models.setEmbeddingRole"
	MethodToolsList              = "tools.list"
	MethodToolsInvoke            = "tools.invoke"

	// Usage reporting (API.md §7.7).
	MethodUsageSession = "usage.session"
	MethodUsageSummary = "usage.summary"

	// Memory / Feedback (API.md §7.7).
	MethodMemoryList     = "memory.list"
	MethodMemoryGet      = "memory.get"
	MethodMemoryUpdate   = "memory.update"
	MethodFeedbackCreate = "feedback.create"

	// Notifications (API.md §7.8) are server→client only.
	NotificationRunEvent       = "notifications.run.event"
	NotificationWorkspaceEvent = "notifications.workspace.event"
)
