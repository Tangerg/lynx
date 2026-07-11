package runs

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// RunStore is the durable Run-admission backstop (§8.2): the authoritative "one
// non-terminal Run per Session" fact that survives restart, distinct from the
// in-memory [Registry] which only tracks THIS process's live segments. The
// composition root injects the sqlite-backed store; a nil store disables the
// durable backstop (the in-memory claim still guards within a single process).
// It covers only run ADMISSION — open a run's durable row, revive a parked run's
// row, reconcile crashed ones at boot; a run's mid-flight state transitions
// (park → interrupted, terminal) ride the atomic [Effects.CommitEvent] alongside
// the interrupt / terminal record they must stay consistent with (§8.3).
//
// Defined here on the consumer side; the sqlite RunStateStore satisfies it
// structurally. It reasons in [execution] admission types, never a backend one.
type RunStore interface {
	// Admit records draft as the session's active (running) Run. It returns
	// [execution.ErrSessionBusy] when the session already has a non-terminal Run
	// — the durable "one non-terminal Run per Session" guarantee (§8.2), upheld
	// across restarts unlike the in-memory [Registry] claim. Called for a fresh
	// root run; a continuation of a parked run calls [RunStore.Resume] instead,
	// reusing the session's existing durable row rather than admitting a second.
	Admit(ctx context.Context, draft execution.RunDraft) error
	// Resume transitions the session's interrupted Run back to running when a
	// parked run continues. Idempotent: a no-op when the row is already running.
	Resume(ctx context.Context, sessionID string) error
	// ReconcileOrphans terminalizes non-terminal Runs abandoned by a crash (their
	// live process is gone after restart and no interrupt keeps them resumable).
	// Run once at boot before admitting any run.
	ReconcileOrphans(ctx context.Context) (int, error)
}
