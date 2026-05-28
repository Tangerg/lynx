// Package rpcadapter is the JSON-RPC ↔ CoreAPI bridge. It owns the
// mapping from JSON-RPC method names (API.md §5.2) to typed CoreAPI
// calls + the inverse encoding of results / errors into JSON-RPC
// envelopes.
//
// Single responsibility:
//
//	Decode  inbound transport.Message → resolve method name → unmarshal params
//	        → call CoreAPI method → marshal result OR map error to code.
//	Encode  CoreAPI streaming events  → notifications/run/event envelopes.
//
// The dispatcher gates business methods behind a successful
// runtime.initialize call — pre-handshake requests get -32011
// protocol_violation. The /v1/info + /v1/health sidecars don't go
// through this dispatcher (they're flat REST endpoints handled by
// pkg/transport/http directly).
package rpcadapter

// JSON-RPC method names — single source for everywhere that needs to
// branch on them. Keeping the names as untyped string constants
// (vs. an enum) matches the JSON wire shape one-to-one and lets the
// HTTP path-cross-check stay a plain string compare.
const (
	// Lifecycle.
	MethodInitialize = "runtime.initialize"
	MethodShutdown   = "runtime.shutdown"
	MethodPing       = "runtime.ping"

	// Runs.
	MethodRunsStart            = "runs.start"
	MethodRunsCancel           = "runs.cancel"
	MethodRunsApprovalSubmit   = "runs.approval.submit"

	// Sessions.
	MethodSessionsList   = "sessions.list"
	MethodSessionsGet    = "sessions.get"
	MethodSessionsCreate = "sessions.create"
	MethodSessionsUpdate = "sessions.update"
	MethodSessionsDelete = "sessions.delete"
	MethodSessionsFork   = "sessions.fork"
	MethodSessionsExport = "sessions.export"

	// Messages.
	MethodMessagesList = "messages.list"
	MethodMessagesEdit = "messages.edit"

	// Workspace.
	MethodWorkspaceFilesChanged       = "workspace.filesChanged"
	MethodWorkspaceDiff               = "workspace.diff"
	MethodWorkspaceFileHead           = "workspace.fileHead"
	MethodWorkspaceGrep               = "workspace.grep"
	MethodWorkspaceTerminalSubscribe  = "workspace.terminal.subscribe"
	MethodWorkspaceProjects           = "workspace.projects"
	MethodWorkspaceMCPList            = "workspace.mcp.list"
	MethodWorkspaceMCPReconnect       = "workspace.mcp.reconnect"
	MethodWorkspaceSkills             = "workspace.skills"

	// Providers / Models / Tools.
	MethodProvidersList = "providers.list"
	MethodProvidersTest = "providers.test"
	MethodModelsList    = "models.list"
	MethodToolsList     = "tools.list"

	// Attachments.
	MethodAttachmentsCreateUploadURL = "attachments.createUploadUrl"
	MethodAttachmentsDelete          = "attachments.delete"

	// Background.
	MethodBackgroundList      = "background.list"
	MethodBackgroundStop      = "background.stop"
	MethodBackgroundSubscribe = "background.subscribe"

	// Feedback.
	MethodFeedbackSubmit = "feedback.submit"

	// Notifications (no response).
	NotificationCancelled       = "notifications/cancelled"
	NotificationRunEvent        = "notifications/run/event"
	NotificationRunClosed       = "notifications/run/closed"
	NotificationBackgroundUpdate = "notifications/background/update"
	NotificationTerminalOutput  = "notifications/terminal/output"
)

