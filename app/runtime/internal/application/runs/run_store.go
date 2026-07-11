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
//
// Defined here on the consumer side; the sqlite RunStateStore satisfies it
// structurally. It reasons in [execution] admission types, never a backend one.
type RunStore interface {
	// Admit records draft as the session's active (running) Run. It returns
	// [execution.ErrSessionBusy] when the session already has a non-terminal Run
	// — the durable "one non-terminal Run per Session" guarantee (§8.2), upheld
	// across restarts unlike the in-memory [Registry] claim.
	Admit(ctx context.Context, draft execution.RunDraft) error
	// Terminalize transitions the session's non-terminal Run to terminal with the
	// given outcome, freeing the session. Idempotent: a no-op when the session has
	// no non-terminal Run.
	Terminalize(ctx context.Context, sessionID, outcome string) error
	// ReconcileOrphans terminalizes running Runs abandoned by a crash (their live
	// process is gone after restart and no interrupt keeps them resumable). Run
	// once at boot before admitting any run.
	ReconcileOrphans(ctx context.Context) (int, error)
}
