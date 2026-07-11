package runtime

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/taskgroup"
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
	tasks taskgroup.Group

	turns     turn.Dispatcher
	closer    io.Closer
	resources []io.Closer
	closeOnce sync.Once
	closeErr  error
	tools     toolsvc.Registry

	approval   approval.Policy
	sessions   sessionsvc.Store
	interrupts interrupts.Store
	transcript transcript.Store

	// history exposes the message-history operations used outside the turn loop
	// — not via the engine (it owns only the steering touchpoint).
	history historyStore

	providers          provider.Registry
	mcpRegistry        mcpserver.Registry
	mcpLiveStatus      mcpLiveStatusReader
	mcpLiveTools       mcpLiveToolCatalog
	mcpLiveConnections mcpLiveConnectionCommands
	mcpLiveRegistry    mcpLiveRegistryCommands
	// mcpMutationMu linearizes the multi-step registry -> live connections ->
	// policy write use case. Locks inside the store and connection adapter cannot
	// protect this cross-component consistency boundary on their own.
	mcpMutationMu sync.Mutex

	// mcpPolicy is atomically replaced after registry changes. Tool resolution
	// and approval read the same immutable domain-policy snapshot.
	mcpPolicy *atomic.Pointer[mcpserver.ToolPolicy]

	defaultProvider string
	defaultModel    string

	// titles auto-names an untitled session from its first user message — a
	// turn-boundary maintenance op (like the Compactor) on the utility model,
	// triggered by the delivery layer off a finished root run.
	titles titleGenerator

	// utility holds the live utility-model role (provider, model) the
	// maintenance services resolve against; SetUtilityRole repoints it.
	// utilityClients validates/builds utility clients; utilStore saves the role
	// across restarts. See utility.go.
	utility        *atomic.Pointer[modelrole.Role]
	utilityClients chatClientResolver
	utilStore      utilityRoleSaver

	// @codebase semantic index: embeddingCell holds the live embedding role,
	// embeddings builds+caches embedders from it, embeddingStore saves it, and
	// codebase is the management/search surface (nil when no CodebaseStore).
	// See embedding.go.
	embeddingCell  *atomic.Pointer[modelrole.Role]
	embeddings     *modelclient.EmbeddingResolver
	embeddingStore embeddingRoleSaver
	codebase       codebaseindex.Index
}
