package runs

import (
	"context"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/completion"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// runCleanupTimeout bounds request-detached Run teardown, so a stuck store or
// executor cannot wedge cancellation.
const runCleanupTimeout = 5 * time.Second

// runTreeOwner is the root Segment's process-local ownership record. It holds the
// lifecycle join, event journal, immutable executor bindings and the root-owned
// cancellation arbiter; specialized behavior lives beside those concerns.
type runTreeOwner struct {
	mu              sync.Mutex
	cancel          context.CancelFunc
	taskContext     context.Context
	hub             *journal
	done            chan struct{}
	completionErr   error
	terminalRun     *run.Run
	terminalRuns    map[string]run.Run
	executorMembers map[string]string
	childCancel     *childCancellation
	cancelRequested bool
	cancelReason    string
	interrupt       interruptBoundary
}

func (owner *runTreeOwner) committedTerminalRun() (run.Run, bool) {
	if owner == nil {
		return run.Run{}, false
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.terminalRun == nil {
		return run.Run{}, false
	}
	return *owner.terminalRun, true
}

// stop cancels the run context. Called on a true terminal (never on a parked
// Run, whose live executor must stay alive for resume).
func (owner *runTreeOwner) stop() {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	cancel := owner.cancel
	owner.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// wait joins the complete run boundary: terminal projection, registry removal,
// synchronous maintenance, admission release, and journal closure.
func (owner *runTreeOwner) wait(ctx context.Context) error {
	if owner == nil || owner.done == nil {
		return nil
	}
	if err := completion.Wait(ctx, owner.done); err != nil {
		return err
	}
	return owner.completionErr
}

// cleanupContext derives a bounded context for a run's durable cancel, rooted on
// the run's detached owner context when available (so cleanup outlives the
// request) and never inheriting the caller's cancellation.
func (owner *runTreeOwner) cleanupContext(fallback context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(fallback)
	if owner != nil {
		owner.mu.Lock()
		if owner.taskContext != nil {
			// The pump can release (and cancel) its task owner immediately after
			// requestCancel stops runCtx. Durable cancel cleanup must retain the
			// owner's trace values without inheriting that lifecycle cancellation.
			base = context.WithoutCancel(owner.taskContext)
		}
		owner.mu.Unlock()
	}
	return context.WithTimeout(base, runCleanupTimeout)
}
