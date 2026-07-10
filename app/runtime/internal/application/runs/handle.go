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
// cancellation, killed only by [Coordinator.Close]), the run's event [Journal],
// and the cancel bookkeeping that linearizes cancellation against interrupt
// publication. It is the [SegmentView] the projector reads at terminal time.
type handle struct {
	mu              sync.Mutex
	cancel          context.CancelFunc
	owner           context.Context
	hub             *Journal[Event]
	cancelRequested bool
	cancelReason    string
}

// requestCancel linearizes cancellation with interrupt publication. Once it
// returns, no new interrupt can be committed for this run; a commit already in
// progress has completed before cancellation proceeds to delete its durable
// record.
func (h *handle) requestCancel(reason string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.cancelRequested = true
	h.cancelReason = reason
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// commitInterrupt runs the durable commit and live publication as one critical
// section relative to requestCancel. committed=false means cancellation won the
// race and commit was deliberately not called.
func (h *handle) commitInterrupt(commit func() error) (committed bool, err error) {
	if h == nil {
		return false, errors.New("runs: missing live run handle")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancelRequested {
		return false, nil
	}
	if err := commit(); err != nil {
		return false, err
	}
	return true, nil
}

// CancelReason returns the recorded human cancel reason — the [SegmentView] seam
// the projector reads when it shapes a canceled terminal. Late-bound on purpose:
// the cancel path sets it after the segment starts.
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

// cleanupContext derives a bounded context for a run's durable cancel, rooted on
// the run's detached owner context when available (so cleanup outlives the
// request) and never inheriting the caller's cancellation.
func (h *handle) cleanupContext(fallback context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(fallback)
	if h != nil {
		h.mu.Lock()
		if h.owner != nil {
			base = h.owner
		}
		h.mu.Unlock()
	}
	return context.WithTimeout(base, runCleanupTimeout)
}
