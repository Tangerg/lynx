package runtime

import (
	"context"
	"io"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// sessionStore is the turn executor's consumer view of session persistence:
// resolve or create the session a turn runs in (Get / Create), record the model
// a run explicitly selected (SetModel), and the terminal auto-titler's
// untitled-only rename (RenameIfUntitled). It is narrower than the sessions
// coordinator's lifecycle surface — the composition root threads the one
// sqlite-backed session store, which satisfies both. Defined here at the
// consumer so the facade names no broad persistence interface.
type sessionStore interface {
	Get(ctx context.Context, id string) (sessionsvc.Session, error)
	Create(ctx context.Context, title, cwd string) (sessionsvc.Session, error)
	SetModel(ctx context.Context, id, model string) error
	RenameIfUntitled(ctx context.Context, id, title string) error
}

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

	sessions   sessionStore
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
