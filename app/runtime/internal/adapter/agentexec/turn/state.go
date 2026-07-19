package turn

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// turnState holds the per-turn bookkeeping the implementation needs:
// the event channel subscribers read from, the cancel func that fires
// when [Dispatcher.Cancel] is called, and a monotone sequence number stamped
// onto every emitted event.
//
// The turn owns its own synchronization: mu guards the cross-goroutine
// mutable state (the backing process, the parked flag, the steering queue),
// reached only through the methods below, so the dispatcher mutex is left to
// guard just the live-turn registry. The remaining fields are set once at
// the entry point and read without locking thereafter.
type turnState struct {
	handle TurnHandle
	events chan Event
	done   chan struct{}
	cancel context.CancelFunc

	// eventMu is the single serialization point for sequence assignment,
	// event delivery, and channel closure. No sender may touch events without
	// it, so endTurn can close the stream without racing a late observer or a
	// park/cancel hand-off.
	eventMu      sync.Mutex
	eventsClosed bool
	eventsOpened bool
	seq          uint64
	terminalOnce sync.Once

	// cwd is the session working directory the turn ran in — threaded to
	// post-turn maintenance so extracted facts land in that project's ledger.
	// Empty only for turns without a session cwd; rehydration receives the
	// canonical cwd from the durable Session and restores it here.
	cwd string

	// hooks is the resolved (trust-filtered) lifecycle-hook set for this turn's
	// cwd, bound once at the entry point. Nil when no hooks apply; every seam
	// calls st.hooks.Run(...) unguarded (the nil Bound no-ops).
	hooks *hooks.Bound

	// ctx is the turn's own lifetime context — derived via
	// context.WithoutCancel from the entry ctx so it outlives the
	// StartTurn caller's cancellation yet KEEPS the entry trace span, then
	// wrapped with the turn span so the engine's LLM / tool / agent spans
	// nest under one trace (full-link). It bounds the run, the resume
	// continuation, and post-turn maintenance; canceled by [Dispatcher.Cancel].
	// Set once at the entry point (StartTurn / Rehydrate).
	ctx context.Context

	// span is the business-level turn span (started at the entry point,
	// ended once in endTurn). Carried on ctx so child spans attach to it.
	span trace.Span

	// model is the resolved model name this turn runs against — stamped on
	// the span + metrics + logs. "default" when the turn didn't pick one.
	model string

	// provider pairs with model to resolve the run's model in the catalog (e.g.
	// its context window for the compaction trigger). "" when the turn uses the
	// default model.
	provider string

	// startedAt stamps the turn's wall-clock start so TurnEnd carries a
	// duration that spans any interrupt/resume cycles.
	startedAt time.Time

	// maxBudget / maxCostUSD echo the turn's configured caps (from the
	// StartTurnRequest), stashed once in runTurn so emitTurnEnd can describe a
	// budget-exceeded terminal precisely. Zero when uncapped.
	maxBudget  int64
	maxCostUSD float64
	maxSteps   int

	// lifecycle captures the process's authoritative terminal event;
	// retained across interrupt→resume so the eventual TurnEnd reads it.
	// Written once before the turn goroutine reads it; not mu-guarded.
	lifecycle *turnLifecycle

	// interruptKinds is the set of HITL kinds the current client can answer
	// for this turn. Nil / empty means no HITL kind may surface.
	interruptKinds map[string]bool

	// --- mu-guarded: mutated/read across the turn + caller goroutines ---
	mu sync.Mutex

	// agentProcess is the process backing this turn, set once setProcess dispatches
	// it. [Dispatcher.Cancel] / [Dispatcher.Resume] / [Dispatcher.ProcessID] read it
	// via process() from other goroutines.
	agentProcess agentexec.TurnProcess

	// startRequest is the immutable request owned by a prepared fresh turn.
	// activated linearizes ActivateTurn against Cancel: exactly one side claims
	// the pre-execution state, so a rejected application admission can tear the
	// turn down without ever entering the model/tool engine.
	startRequest StartTurnRequest
	prepared     bool
	activated    bool

	// parked is true while the turn is suspended on a HITL interrupt
	// (StatusWaiting) awaiting [Dispatcher.Resume]. A parked turn stays
	// registered (events channel open) until claimPark drives it to a
	// terminal state.
	parked bool

	// steering is the queue of mid-turn user messages injected via
	// [Dispatcher.InjectSteering]. The runtime flushes it to the chat history
	// store after the turn ends so the messages land in conversation
	// history for the next turn.
	steering []string

	// flushed marks the steering queue closed — the turn has committed to
	// terminating and run its final flushSteering, so no future round will drain
	// the queue again. Once set, appendSteering rejects (ErrTurnNotFound): a
	// steer that races turn-end must bounce back to the client (which retries it
	// as a fresh send) rather than be queued into a turn nothing will ever drain.
	flushed bool
}

func (st *turnState) prepareStart(request StartTurnRequest) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.startRequest = request
	st.prepared = true
}

func (st *turnState) claimStart() (StartTurnRequest, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.prepared || st.activated {
		return StartTurnRequest{}, false
	}
	st.activated = true
	request := st.startRequest
	st.startRequest = StartTurnRequest{}
	return request, true
}

func (st *turnState) cancelPrepared() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.prepared || st.activated {
		return false
	}
	st.activated = true
	st.startRequest = StartTurnRequest{}
	return true
}

// newTurnState builds a fresh per-turn state. Its lifetime ctx derives from the
// entry ctx via context.WithoutCancel: the caller's ctx ending (e.g. the
// StartTurn RPC returning) doesn't kill the in-flight turn; only
// [Dispatcher.Cancel] (st.cancel) does; yet the entry trace span is preserved,
// so the engine's spans chain onto the same trace. The turn span is layered on
// in StartTurn / Rehydrate. Shared by both entry points so they produce an
// identically-initialized turn.
func newTurnState(ctx context.Context, handle TurnHandle) *turnState {
	lifeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &turnState{
		handle:    handle,
		events:    make(chan Event, 32),
		done:      make(chan struct{}),
		cancel:    cancel,
		ctx:       lifeCtx,
		startedAt: time.Now(),
	}
}

func (st *turnState) setInterruptKinds(kinds []string) {
	st.interruptKinds = make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		st.interruptKinds[kind] = true
	}
}

func (st *turnState) claimEvents() bool {
	st.eventMu.Lock()
	defer st.eventMu.Unlock()
	if st.eventsOpened {
		return false
	}
	st.eventsOpened = true
	return true
}

func (st *turnState) canSurface(kind string) bool {
	return st.interruptKinds[kind]
}

// setProcess records the agent process backing this turn. runTurn / Rehydrate
// write it once they have dispatched one; process() then hands it to the
// caller goroutines.
func (st *turnState) setProcess(process agentexec.TurnProcess) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.agentProcess = process
}

// process returns the backing agent process, or nil before the turn has
// dispatched one. The value is stable after the single setProcess, so callers
// may invoke its methods after process() returns.
func (st *turnState) process() agentexec.TurnProcess {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.agentProcess
}

// parkIfLive marks the turn suspended on a HITL interrupt awaiting Resume,
// unless its ctx was already canceled — the atomic guard that closes the
// Cancel-vs-parking race. A Cancel racing a turn that's about to park either
// (a) runs claimPark after parked is set here, so its claim wins and it
// finishes the turn, or (b) cancels the ctx before this acquires mu, so this
// returns false and the caller finishes instead. Because both this and
// claimPark hold st.mu, they can't interleave, so exactly one path drives the
// turn to a terminal — never an orphan parked turn on a dead ctx that no one
// finishes. Returns false when the turn must NOT park (already canceled), true
// when it is now parked.
func (st *turnState) parkIfLive() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.ctx.Err() != nil {
		return false
	}
	st.parked = true
	return true
}

// claimPark atomically tests-and-clears the parked flag, reporting whether
// THIS caller claimed the suspended turn. [Dispatcher.Resume] and [Dispatcher.Cancel]
// both race to act on a parked turn; whoever flips the flag false wins and owns
// driving it to a terminal state, so the loser is a no-op. Returns false for a
// turn that isn't parked (never suspended, or already claimed).
func (st *turnState) claimPark() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.parked {
		return false
	}
	st.parked = false
	return true
}

// appendSteering pushes one user message onto the pending-steering queue, or
// returns [ErrTurnNotFound] when the queue is already closed (the turn is
// terminating — see [turnState.flushed]) so the caller treats it like steering a
// turn that has ended.
func (st *turnState) appendSteering(message string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.flushed {
		return ErrTurnNotFound
	}
	st.steering = append(st.steering, message)
	return nil
}

// drainSteering returns the queued steering messages and clears the queue,
// or nil when none is pending. Used by the mid-run steerSource (the queue stays
// open for further rounds); the terminal flush uses closeAndDrainSteering.
func (st *turnState) drainSteering() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.steering) == 0 {
		return nil
	}
	out := st.steering
	st.steering = nil
	return out
}

// closeAndDrainSteering closes the queue (the turn is terminating; no later
// round will drain it) and returns the pending messages — atomically, so a
// steer racing turn-end is either captured by this final drain or rejected by
// the now-closed appendSteering, never queued into a turn nothing will drain.
func (st *turnState) closeAndDrainSteering() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.flushed = true
	out := st.steering
	st.steering = nil
	return out
}
