package runtime

import (
	"io"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Runtime is the residual execution facade: the turn/engine surface (the
// runs.Executor the run pump drives) plus the durable session/transcript/history
// stores it reads for turn planning and projections. Batch 5 relocates this
// surface to adapter/agentexec behind an Executor port. Construct once via [New].
//
// Concurrency: every dependency Runtime exposes owns its own synchronization.
// Runtime owns the process-local task group backing the run pump's post-commit
// work (a run-lifecycle concern that moves to the RunSupervisor in Batch 5).
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

	// titles auto-names an untitled session from its first user message — a
	// turn-boundary maintenance op (like the Compactor) on the utility model,
	// triggered by the delivery layer off a finished root run.
	titles titleGenerator
}
