package runs

import (
	"context"
	"errors"
	"sync"
	"time"
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
	mu                sync.Mutex
	cancel            context.CancelFunc
	owner             context.Context
	hub               *Journal
	done              chan struct{}
	cancelRequested   bool
	cancelReason      string
	inflightInterrupt *interruptCommit
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
func (h *handle) requestCancel(reason string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.cancelRequested = true
	h.cancelReason = reason
	cancelRun := h.cancel
	inflight := h.inflightInterrupt
	h.mu.Unlock()
	if cancelRun != nil {
		cancelRun()
	}
	if inflight != nil {
		inflight.cancel()
	}
	if inflight != nil {
		<-inflight.done
	}
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
	if h.inflightInterrupt != nil {
		h.mu.Unlock()
		cancelCommit()
		return false, errors.New("runs: interrupt commit already in flight")
	}
	inflight := &interruptCommit{done: make(chan struct{}), cancel: cancelCommit}
	h.inflightInterrupt = inflight
	h.mu.Unlock()

	err = commit(commitCtx)
	cancelCommit()
	h.mu.Lock()
	close(inflight.done)
	h.inflightInterrupt = nil
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
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
