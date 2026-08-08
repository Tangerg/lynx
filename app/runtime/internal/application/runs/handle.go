package runs

import (
	"context"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/completion"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// runCleanupTimeout bounds request-detached Run teardown, so a stuck store or
// executor cannot wedge cancellation.
const runCleanupTimeout = 5 * time.Second

// handle is the root segment's process-local ownership record. It holds the
// lifecycle join, event Journal, immutable executor bindings and the root-owned
// cancellation arbiter; specialized behavior lives beside those concerns.
type handle struct {
	mu              sync.Mutex
	cancel          context.CancelFunc
	owner           context.Context
	hub             *Journal
	done            chan struct{}
	completionErr   error
	terminalRun     *transcript.Run
	terminalRuns    map[string]transcript.Run
	executorMembers map[string]string
	childCancel     *childCancellation
	cancelRequested bool
	cancelReason    string
	interrupt       interruptBoundary
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

// stop cancels the run context. Called on a true terminal (never on a parked
// Run, whose live executor must stay alive for resume).
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
