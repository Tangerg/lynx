package runtime

import (
	"context"
	"io"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
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

// transcriptStore is the facade's view of the durable transcript: the two read
// projections it serves (List / ListRuns — items.list and the run timeline) plus
// the run-segment committer's append writes (AppendItem / PutRun). Narrower than
// the sessions coordinator's lifecycle transcript surface; the composition root
// threads the one sqlite-backed transcript store, which satisfies both. Defined
// here at the consumer so the facade names no broad persistence interface.
type transcriptStore interface {
	List(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
	ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
	AppendItem(ctx context.Context, it transcript.Item) error
	PutRun(ctx context.Context, r transcript.Run) error
}

// interruptStore is the facade's view of the open-interrupt registry: listing a
// session's open interrupts (List) and the run-segment committer's park write
// (Put). Narrower than the sessions coordinator's resume/cancel surface.
type interruptStore interface {
	List(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
	Put(ctx context.Context, p interrupts.Pending) error
}

// runStateWriter is the facade's view of the durable Run-admission state the
// run-segment committer transitions inside the event commit (§8.3): a park
// suspends the run, a terminal terminalizes it. Narrower than the run
// coordinator's admission surface; the one sqlite-backed store satisfies both.
type runStateWriter interface {
	Suspend(ctx context.Context, sessionID string) error
	Terminalize(ctx context.Context, sessionID, outcome string) error
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
	interrupts interruptStore
	transcript transcriptStore

	// runState + transact back the run-segment committer's atomic event commit:
	// the durable Run-state transition and the transactional seam that lands it in
	// one transaction with the interrupt / transcript record (§8.3).
	runState runStateWriter
	transact Transactor

	// history exposes the message-history operations used outside the turn loop
	// — not via the engine (it owns only the steering touchpoint).
	history historyStore

	// titles auto-names an untitled session from its first user message — a
	// turn-boundary maintenance op (like the Compactor) on the utility model,
	// triggered by the delivery layer off a finished root run.
	titles titleGenerator
}
