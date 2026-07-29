package runs

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/completion"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// runCleanupTimeout bounds the request-detached work that tears a run down /
// cancels its turn, so a stuck store or agent can't wedge cancellation.
const runCleanupTimeout = 5 * time.Second

// handle holds the coordinator-owned resources for one in-flight run segment:
// the run context's cancel, the detached owner context (survives request
// cancellation, canceled by [Coordinator.BeginShutdown]), the run's event
// [Journal], its terminal join point, and the cancel bookkeeping that linearizes
// cancellation against interrupt publication. The reducer reads its late-bound
// cancellation reason.
type handle struct {
	mu              sync.Mutex
	cancel          context.CancelFunc
	owner           context.Context
	hub             *Journal
	done            chan struct{}
	completionErr   error
	terminalRun     *transcript.Run
	cancelRequested bool
	cancelReason    string
	interrupt       interruptBoundary
}

// recordTerminalRun retains the exact Run snapshot whose terminal transaction
// just committed. Cancel reads this after joining done, so its result cannot
// drift from the write-set it acknowledges.
func (h *handle) recordTerminalRun(run transcript.Run) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.terminalRun != nil {
		panic("runs: live segment committed more than one terminal run")
	}
	h.terminalRun = &run
}

func (h *handle) committedTerminalRun() (transcript.Run, bool) {
	if h == nil {
		return transcript.Run{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.terminalRun == nil {
		return transcript.Run{}, false
	}
	return *h.terminalRun, true
}

// interruptBoundary records the only two durable facts cancellation needs:
// whether publication committed, and whether one publication is currently
// owned. Both fields are guarded by handle.mu.
type interruptBoundary struct {
	committed bool
	active    *interruptCommit
}

// interruptCommit is the one cancellable interrupt publication a run may own.
// A nil pointer means there is no commit to interrupt or join.
type interruptCommit struct {
	done   chan struct{}
	cancel context.CancelFunc
}

// requestCancel linearizes cancellation with interrupt publication. Once it
// returns, no new interrupt can be committed for this run; a commit already in
// progress has observed cancellation and completed before cancellation proceeds.
// External I/O never runs under mu: the in-flight channel is the join point.
func (h *handle) requestCancel(ctx context.Context, reason string) (interruptCommitted bool, err error) {
	if h == nil {
		return false, nil
	}
	h.mu.Lock()
	h.cancelRequested = true
	h.cancelReason = reason
	cancelRun := h.cancel
	inflight := h.interrupt.active
	h.mu.Unlock()
	if cancelRun != nil {
		cancelRun()
	}
	if inflight != nil {
		inflight.cancel()
	}
	if inflight != nil {
		select {
		case <-inflight.done:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	h.mu.Lock()
	committed := h.interrupt.committed
	h.mu.Unlock()
	return committed, nil
}

// commitInterrupt reserves the interrupt boundary, runs its context-bounded
// durable commit and publication without holding mu, then releases waiting
// cancellation. committed=false means cancellation won before the reservation
// or the commit failed.
func (h *handle) commitInterrupt(ctx context.Context, commit func(context.Context) error) (committed bool, err error) {
	if h == nil {
		return false, errors.New("runs: missing live run handle")
	}
	commitCtx, cancelCommit := context.WithTimeout(ctx, runCleanupTimeout)
	h.mu.Lock()
	if h.cancelRequested {
		h.mu.Unlock()
		cancelCommit()
		return false, nil
	}
	if h.interrupt.committed || h.interrupt.active != nil {
		h.mu.Unlock()
		cancelCommit()
		return false, errors.New("runs: interrupt boundary already resolved")
	}
	inflight := &interruptCommit{done: make(chan struct{}), cancel: cancelCommit}
	h.interrupt.active = inflight
	h.mu.Unlock()

	err = commit(commitCtx)
	cancelCommit()
	h.mu.Lock()
	if err == nil {
		h.interrupt.committed = true
	}
	close(inflight.done)
	h.interrupt.active = nil
	h.mu.Unlock()
	if err != nil {
		return false, err
	}
	return true, nil
}

// CancelReason returns the recorded human cancel reason. It is late-bound on
// purpose because cancellation can arrive after the segment starts.
func (h *handle) CancelReason() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cancelReason
}

// stop cancels the run context. Called on a true terminal (never on a parked
// run, whose live turn must stay alive for resume).
func (h *handle) stop() {
	if h == nil {
		return
	}
	h.mu.Lock()
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// wait joins the complete run boundary: terminal projection, registry removal,
// synchronous maintenance, admission release, and Journal closure.
func (h *handle) wait(ctx context.Context) error {
	if h == nil || h.done == nil {
		return nil
	}
	if err := completion.Wait(ctx, h.done); err != nil {
		return err
	}
	return h.completionErr
}

// cleanupContext derives a bounded context for a run's durable cancel, rooted on
// the run's detached owner context when available (so cleanup outlives the
// request) and never inheriting the caller's cancellation.
func (h *handle) cleanupContext(fallback context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(fallback)
	if h != nil {
		h.mu.Lock()
		if h.owner != nil {
			// The pump can release (and cancel) its task owner immediately after
			// requestCancel stops runCtx. Durable cancel cleanup must retain the
			// owner's trace values without inheriting that lifecycle cancellation.
			base = context.WithoutCancel(h.owner)
		}
		h.mu.Unlock()
	}
	return context.WithTimeout(base, runCleanupTimeout)
}
