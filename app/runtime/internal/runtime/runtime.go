package runtime

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Runtime is the residual execution facade: the turn/engine surface (the
// runs.Executor the run pump drives), the durable session/transcript/history
// stores it reads for turn planning and projections, and the MCP + codebase
// live-capability control still awaiting extraction. Construct once via [New].
//
// Concurrency: every dependency Runtime exposes owns its own synchronization.
// Runtime owns the process-local coordination state (the request-detached task
// group) that defines application lifecycle invariants across transports.
type Runtime struct {
	tasks taskgroup.Group

	turns     turn.Dispatcher
	closer    io.Closer
	resources []io.Closer
	closeOnce sync.Once
	closeErr  error

	sessions   sessionsvc.Store
	interrupts interrupts.Store
	transcript transcript.Store

	// history exposes the message-history operations used outside the turn loop
	// — not via the engine (it owns only the steering touchpoint).
	history historyStore

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

	// titles auto-names an untitled session from its first user message — a
	// turn-boundary maintenance op (like the Compactor) on the utility model,
	// triggered by the delivery layer off a finished root run.
	titles titleGenerator

	// codebase is the @codebase semantic index management/search surface (nil
	// when no CodebaseStore); the embedding role that drives it is owned by the
	// capabilities coordinator, which shares the live cell this index reads.
	codebase codebaseindex.Index
}
