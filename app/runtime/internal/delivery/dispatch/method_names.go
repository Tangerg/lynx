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
// The dispatcher gates business methods behind a successful
// runtime.initialize call — pre-handshake requests get
// capability_not_negotiated. The /v2/info + /v2/health sidecars don't
// go through this dispatcher (flat REST handled by delivery/transport/http).
package dispatch

// JSON-RPC method names — single source for everywhere that needs to
// branch on them. Untyped string constants match the JSON wire shape
// one-to-one and let the HTTP path-cross-check stay a plain compare.
// Names are dotted <domain>.<verb> (API.md §2.3); HTTP keeps the dots.
const (
	// Lifecycle (API.md §7.1).
	MethodInitialize = "runtime.initialize"
	MethodShutdown   = "runtime.shutdown"
	MethodPing       = "runtime.ping"

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
	MethodWorkspaceListSkills      = "workspace.listSkills"
	MethodWorkspaceListAgentDocs   = "workspace.listAgentDocs"
	MethodWorkspaceMCPListServers  = "workspace.mcp.listServers"
	MethodWorkspaceMCPListTools    = "workspace.mcp.listTools"
	MethodWorkspaceMCPReconnect    = "workspace.mcp.reconnect"
	MethodWorkspaceMCPAuthorize    = "workspace.mcp.authorize"
	MethodWorkspaceMCPListConfigs  = "workspace.mcp.listConfigs"
	MethodWorkspaceMCPConfigure    = "workspace.mcp.configure"
	MethodWorkspaceMCPRemove       = "workspace.mcp.remove"
	MethodWorkspaceMCPSetEnabled   = "workspace.mcp.setEnabled"
	MethodWorkspaceMCPTest         = "workspace.mcp.test"
	MethodWorkspaceListHooks       = "workspace.hooks.list"
	MethodWorkspaceSetHookTrust    = "workspace.hooks.setTrust"
	MethodWorkspaceSubscribe       = "workspace.subscribe"

	// Approval (API.md §C.3) — runtime-global tool-permission stance + the
	// persistent fine-grained "remember this decision" rules.
	MethodApprovalGetMode    = "approval.getMode"
	MethodApprovalSetMode    = "approval.setMode"
	MethodApprovalListRules  = "approval.listRules"
	MethodApprovalForgetRule = "approval.forgetRule"

	// Providers / Models / Tools (API.md §7.6).
	MethodProvidersList        = "providers.list"
	MethodProvidersConfigure   = "providers.configure"
	MethodProvidersTest        = "providers.test"
	MethodModelsList           = "models.list"
	MethodModelsGetUtilityRole = "models.getUtilityRole"
	MethodModelsSetUtilityRole = "models.setUtilityRole"
	MethodToolsList            = "tools.list"
	MethodToolsInvoke          = "tools.invoke"

	// Usage reporting (API.md §7.7).
	MethodUsageSession = "usage.session"
	MethodUsageSummary = "usage.summary"

	// Memory / Feedback (API.md §7.7).
	MethodMemoryList     = "memory.list"
	MethodMemoryGet      = "memory.get"
	MethodMemoryUpdate   = "memory.update"
	MethodFeedbackCreate = "feedback.create"

	// Notifications (API.md §7.8). notifications.canceled is
	// client→server; the rest are server→client.
	NotificationCanceled       = "notifications.canceled"
	NotificationRunEvent       = "notifications.run.event"
	NotificationWorkspaceEvent = "notifications.workspace.event"
)
