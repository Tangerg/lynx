package runtime

import (
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Runtime is the bundle. Construct once via [New]; share the
// pointer across every transport adapter that needs to dispatch
// turns / sessions / approvals.
//
// Concurrency: every dependency Runtime exposes owns its own synchronization.
// Runtime owns the process-local coordination state that defines application
// lifecycle invariants across transports.
type Runtime struct {
	engine     *kernel.Engine
	turns      turn.Dispatcher
	tools      toolsvc.Registry
	knowledge  knowledge.Store
	approval   approval.Policy
	interrupts interrupts.Store
	transcript transcript.Store

	sessionList       sessionList
	sessionRead       sessionRead
	sessionCreation   sessionCreate
	sessionPatch      sessionPatchWriter
	sessionModel      sessionModelWriter
	sessionLifecycle  lifecycle.SessionStore
	sessionRunSegment runsegment.SessionStore

	// conversation is the message history the non-turn history ops
	// (ReadHistory/SeedHistory/MessageCount/TruncateMessages) delegate to
	// directly — not via the engine (it owns only the steering touchpoint).
	conversation *conversation.Messages

	providerRegistryList      providerRegistryList
	providerRegistryRead      providerRegistryRead
	providerRegistryConfigure providerRegistryConfigure

	mcpRegistryList      mcpServerList
	mcpRegistryRead      mcpServerRead
	mcpRegistryConfigure mcpServerConfigure
	mcpRegistryRemove    mcpServerRemove
	mcpRegistryEnable    mcpServerEnable

	// mcpGating holds the current per-call MCP tool gating (disabled / auto-
	// approve sets), recomputed on every registry change. The resolver (disabled
	// filter) and the turn gate (auto-approve skip) read it via closures that
	// close over this same cell, captured at construction before the Runtime
	// exists — hence a pointer. See [mcpGating] and [Runtime.refreshMCPGating].
	mcpGating *atomic.Pointer[mcpGating]

	defaultProvider string
	defaultModel    string

	// titler auto-names an untitled session from its first user message — a
	// turn-boundary maintenance op (like the Compactor) on the utility model,
	// triggered by the delivery layer off a finished root run.
	titler *maintenance.Titler

	// utility holds the live utility-model role (provider, model) the
	// maintenance services resolve against; SetUtilityRole repoints it. resolver
	// builds + caches the client for a (provider, model); utilStore persists the
	// role across restarts. See utility.go.
	utility   *atomic.Pointer[utilityRole]
	resolver  *clientResolver
	utilStore UtilityRoleStore

	// hookResolver inspects lifecycle hooks for a cwd (workspace.hooks.list);
	// hookTrust mutates project trust (workspace.hooks.setTrust). Both nil when
	// hooks are unconfigured.
	hookResolver HookResolver
	hookTrust    HookTrustStore

	// recipesGlobalDir is the global recipes directory the workspace.recipes.list
	// discovery layers under a project's .lyra/recipes. Empty → project-only.
	recipesGlobalDir string

	// schedules.* ports. nil when scheduling is unconfigured.
	scheduleList     scheduleList
	scheduleRead     scheduleRead
	scheduleCreation scheduleCreate
	scheduleUpdates  scheduleUpdate
	scheduleDeletion scheduleDelete
	scheduleRuns     scheduleRunRecorder
	scheduleWorker   schedule.WorkerStore

	// @codebase semantic index: embeddingCell holds the live embedding role,
	// embeddings builds+caches embedders from it, embeddingStore persists it, and
	// codebaseIndex is the index (nil when no CodebaseStore). See
	// embedding.go.
	embeddingCell  *atomic.Pointer[embeddingRole]
	embeddings     *embeddingResolver
	embeddingStore EmbeddingRoleStore
	codebaseIndex  codebaseindex.Index

	// transactor runs a write-set inside one storage transaction so the
	// cross-store operations (sessions.import / rollback) are atomic; nil → run
	// directly (RunInTx). See [Transactor].
	transactor Transactor

	// workingTrees coordinates short run admissions with destructive
	// working-tree mutations for every transport using this runtime.
	workingTrees lifecycle.WorkingTreeGate
}
