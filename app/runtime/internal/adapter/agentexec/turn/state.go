package turn

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// turnState holds the per-turn bookkeeping the implementation needs:
// the event channel subscribers read from, the cancel func that fires
// when Cancel is called, and a monotone sequence number stamped
// onto every emitted event.
//
// The turn owns its own synchronization: mu guards the cross-goroutine
// mutable state (the backing process, lifecycle phase, the steering queue),
// reached only through the methods below, so the dispatcher mutex is left to
// guard just the live-turn registry. The remaining fields are set once at
// the entry point and read without locking thereafter.
type turnState struct {
	handle TurnHandle
	events chan runs.ExecutorEvent
	done   chan struct{}
	cancel context.CancelFunc

	// eventMu is the single serialization point for sequence assignment,
	// event delivery, and channel closure. No sender may touch events without
	// it, so terminalization can close the stream without racing a late observer
	// or a park/cancel hand-off.
	eventMu      sync.Mutex
	eventsClosed bool

	// lifecycleMu serializes external lifecycle commands (process cancellation
	// and terminal discard). The phase itself lives under mu so process
	// publication and Resume can make short atomic transitions without holding a
	// lock across engine I/O.
	lifecycleMu sync.Mutex

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
	// continuation, and post-turn maintenance; canceled by Cancel.
	// Set once at the entry point (StartTurn / Rehydrate).
	ctx context.Context

	// span is the business-level turn span (started at the entry point and ended
	// once terminal process ownership is released). Carried on ctx so child
	// spans attach to it.
	span trace.Span

	// model is the resolved model name this turn runs against — stamped on
	// the span + metrics + logs. "default" when the turn didn't pick one.
	model string

	// modelSelection remains the canonical per-run choice for maintenance and
	// recovery. model above is its observability projection ("default" when unset).
	modelSelection modelref.Selection

	// interruptKinds is the set of HITL kinds the current client can answer
	// for this turn. Nil / empty means no HITL kind may surface.
	interruptKinds map[execution.InterruptKind]struct{}

	// --- mu-guarded: mutated/read across the turn + caller goroutines ---
	mu sync.Mutex

	// segmentStartedAt stamps when the CURRENT segment began executing, so the
	// duration reported to the run is active time. It is re-stamped on resume:
	// a turn spanning an interrupt would otherwise report the hours a person
	// took to answer as time the model spent working.
	segmentStartedAt time.Time

	// phase is the complete execution-ownership state machine. A single phase
	// replaces independent prepared/parked/canceling/terminal/released flags, so
	// impossible combinations cannot be represented. lifecycleChanged wakes a
	// shutdown owner when late process publication or terminal cleanup makes new
	// progress possible.
	phase            turnPhase
	lifecycleChanged chan struct{}

	// Exactly one segment consumes events at a time. A parked turn releases its
	// active consumer so the continuation segment can take over; a terminal turn
	// permits only its first (possibly late) drain.
	eventsActive  bool
	eventsClaimed bool
	eventsEnded   bool

	// agentProcess is the process backing this turn, set once setProcess dispatches
	// it. Cancel, Resume, and ProcessID read it
	// via process() from other goroutines.
	agentProcess agentexec.TurnProcess

	// startRequest is the immutable request owned by a prepared fresh turn.
	// turnPrepared linearizes ActivateTurn against Cancel: exactly one side
	// claims the pre-execution state, so a rejected application admission can
	// tear the turn down without ever entering the model/tool engine.
	startRequest runs.StartTurn

	// steering is the queue of validated mid-turn user messages injected via
	// InjectSteering. Each entry retains both canonical transcript content and
	// the provider-neutral model message so the live and terminal-fallback paths
	// consume the same interpretation exactly once.
	steering []steeringMessage

	// flushed marks the steering queue closed — the turn has committed to
	// terminating and run its final flushSteering, so no future round will drain
	// the queue again. Once set, appendSteering rejects (ErrTurnNotFound): a
	// steer that races turn-end must bounce back to the client (which retries it
	// as a fresh send) rather than be queued into a turn nothing will ever drain.
	flushed bool

	// doom-loop brake (T13): track the last completed tool call so a model stuck
	// repeating the SAME call with NO new information can be halted. doomKey is
	// the call's tool+arguments; doomResult its output hash; doomRepeat the count
	// of consecutive identical calls whose output was also identical. A repeated
	// call whose output changes (e.g. polling a background command that is making
	// progress) resets the count, so only a genuine no-progress loop is braked.
	doomKey    string
	doomResult uint64
	doomRepeat int

	// toolCalls counts completed root-process tool calls — the skill miner's
	// complexity signal. Child observations carry process ownership and are not
	// projected while the protocol has no child-run model, so they cannot skew a
	// root transcript's complexity score.
	toolCalls int
}

type turnPhase uint8

const (
	turnPreparing turnPhase = iota
	turnPrepared
	turnStarting
	turnRunning
	turnParked
	turnResuming
	turnRestoring
	// turnCancelDriven means a start/run/resume owner will publish the terminal.
	turnCancelDriven
	// turnCancelIdle means no drive goroutine owns completion; Cancel must finish
	// a prepared turn or terminate a restored/parked process.
	turnCancelIdle
	turnTerminal
	turnReleased
)

type cancellationAction uint8

const (
	cancelObserve cancellationAction = iota
	cancelFinish
	cancelProcess
	cancelComplete
)

func (st *turnState) completePreparation(request runs.StartTurn) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.phase != turnPreparing {
		panic("turn: complete preparation outside preparing phase")
	}
	st.startRequest = request
	st.setPhaseLocked(turnPrepared)
}

func (st *turnState) claimStart() (runs.StartTurn, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.phase != turnPrepared {
		return runs.StartTurn{}, false
	}
	request := st.startRequest
	st.startRequest = runs.StartTurn{}
	st.setPhaseLocked(turnStarting)
	return request, true
}

func (st *turnState) requestCancellation() cancellationAction {
	st.mu.Lock()
	defer st.mu.Unlock()
	switch st.phase {
	case turnPrepared:
		st.startRequest = runs.StartTurn{}
		st.setPhaseLocked(turnCancelIdle)
		return cancelFinish
	case turnRestoring:
		st.setPhaseLocked(turnCancelIdle)
		return cancelObserve
	case turnParked:
		st.setPhaseLocked(turnCancelIdle)
		return cancelProcess
	case turnStarting, turnRunning, turnResuming:
		st.setPhaseLocked(turnCancelDriven)
		return cancelObserve
	case turnCancelIdle:
		if st.agentProcess != nil {
			return cancelProcess
		}
		return cancelObserve
	case turnCancelDriven:
		return cancelObserve
	case turnTerminal, turnReleased:
		return cancelComplete
	default:
		panic("turn: cancellation requested in a non-addressable lifecycle phase")
	}
}

// newPreparingTurnState builds a fresh start state before prompt hooks finish.
func newPreparingTurnState(ctx context.Context, handle TurnHandle) *turnState {
	return newTurnState(ctx, handle, turnPreparing)
}

// newRestoringTurnState builds a state whose process publication is owned by
// Rehydrate.
func newRestoringTurnState(ctx context.Context, handle TurnHandle) *turnState {
	return newTurnState(ctx, handle, turnRestoring)
}

// newTurnState initializes common per-turn ownership at one explicit entry
// phase. Its lifetime ctx derives from the
// entry ctx via context.WithoutCancel: the caller's ctx ending (e.g. the
// StartTurn RPC returning) doesn't kill the in-flight turn; only
// Cancel (st.cancel) does; yet the entry trace span is preserved,
// so the engine's spans chain onto the same trace. The turn span is layered on
// in StartTurn / Rehydrate. Shared by both entry points so they produce an
// identically-initialized turn.
func newTurnState(ctx context.Context, handle TurnHandle, phase turnPhase) *turnState {
	lifeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &turnState{
		handle:           handle,
		events:           make(chan runs.ExecutorEvent, 32),
		done:             make(chan struct{}),
		cancel:           cancel,
		ctx:              lifeCtx,
		segmentStartedAt: time.Now(),
		phase:            phase,
		lifecycleChanged: make(chan struct{}),
	}
}

func (st *turnState) setInterruptKinds(kinds []execution.InterruptKind) {
	st.interruptKinds = make(map[execution.InterruptKind]struct{}, len(kinds))
	for _, kind := range kinds {
		if kind.Valid() {
			st.interruptKinds[kind] = struct{}{}
		}
	}
}

func (st *turnState) claimEvents() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.eventsActive || (st.eventsClaimed && st.eventsEnded) {
		return false
	}
	st.eventsActive = true
	st.eventsClaimed = true
	return true
}

func (st *turnState) releaseEvents() {
	st.mu.Lock()
	st.eventsActive = false
	st.mu.Unlock()
}

func (st *turnState) canSurface(kind execution.InterruptKind) bool {
	_, ok := st.interruptKinds[kind]
	return ok
}

// recordToolOutcome folds one completed tool call into the doom-loop counter.
// It is called from the tool-observer end callback (possibly concurrently for a
// parallel round), so it takes mu. arguments are the effective post-approval
// arguments — the same value the gate reads via repeatedNoProgress — so the two
// sides key consistently.
func (st *turnState) recordToolOutcome(toolName, arguments, output string) {
	key := toolName + "\x00" + arguments
	digest := hashOutput(output)
	st.mu.Lock()
	defer st.mu.Unlock()
	// Count every completed call for the miner's complexity signal, independent
	// of the doom-loop no-progress fold below.
	st.toolCalls++
	if key == st.doomKey && digest == st.doomResult {
		st.doomRepeat++
		return
	}
	st.doomKey = key
	st.doomResult = digest
	st.doomRepeat = 1
}

// repeatedNoProgress reports how many times the last run of completed calls was
// exactly this tool+arguments with an unchanging output. The gate reads it
// BEFORE running the call, so a return >= the brake threshold means enough
// identical no-progress calls have already completed to treat the next as a loop.
func (st *turnState) repeatedNoProgress(toolName, arguments string) int {
	key := toolName + "\x00" + arguments
	st.mu.Lock()
	defer st.mu.Unlock()
	if key != st.doomKey {
		return 0
	}
	return st.doomRepeat
}

// resetDoomLoop clears the no-progress streak once the brake has escalated a
// call to a human. The escalation parks the turn, and an in-memory resume reuses
// this same turnState, so without an explicit reset the very next identical call
// would re-trip the brake — the model would never get room to continue after the
// human's decision. Zeroing the count here gives it a fresh run of calls.
func (st *turnState) resetDoomLoop() {
	st.mu.Lock()
	st.doomRepeat = 0
	st.mu.Unlock()
}

// toolCallCount reports how many tool calls have completed this turn — the
// skill miner's complexity signal, read once at the turn boundary.
func (st *turnState) toolCallCount() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.toolCalls
}

// hashOutput is a cheap, allocation-free fingerprint of a tool result. Only
// equality matters (did the output change between identical calls), so FNV-64a
// is sufficient and collisions merely risk one missed brake, never corruption.
func hashOutput(output string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(output))
	return h.Sum64()
}

// setProcess publishes the process owned by a fresh start. The drive goroutine
// remains the terminal owner even when cancellation won before publication.
func (st *turnState) setProcess(process agentexec.TurnProcess) {
	st.mu.Lock()
	defer st.mu.Unlock()
	switch st.phase {
	case turnStarting:
		st.agentProcess = process
		st.setPhaseLocked(turnRunning)
	case turnCancelDriven:
		st.agentProcess = process
		st.signalLifecycleLocked()
	default:
		panic("turn: fresh process published outside starting phase")
	}
}

// setRestoredProcess publishes a restored process and its parked ownership
// atomically. A concurrent Cancel either transitions restoring→cancel-idle or
// observes the parked process after this method; exactly one lifecycle owner
// then tears it down.
func (st *turnState) setRestoredProcess(process agentexec.TurnProcess) (live bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	switch st.phase {
	case turnRestoring:
		st.agentProcess = process
		st.setPhaseLocked(turnParked)
		return st.ctx.Err() == nil
	case turnCancelIdle:
		st.agentProcess = process
		st.signalLifecycleLocked()
		return false
	default:
		panic("turn: restored process published outside restoring phase")
	}
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
	if st.ctx.Err() != nil || st.phase != turnRunning {
		return false
	}
	st.setPhaseLocked(turnParked)
	return true
}

// claimPark transfers a parked process to the synchronous Resume operation.
// Cancel transitions the same phase to turnCancelIdle; only one transition can
// win.
func (st *turnState) claimPark() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.phase != turnParked {
		return false
	}
	st.setPhaseLocked(turnResuming)
	return true
}

func (st *turnState) resumeStarted() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.phase == turnResuming {
		// A continuation is a new segment, and its clock starts here. (A rehydrated
		// turn arrives with a freshly constructed state, so its stamp is already
		// this resume.)
		st.segmentStartedAt = time.Now()
		st.setPhaseLocked(turnRunning)
	}
}

// segmentElapsed is how long the current segment has been executing.
func (st *turnState) segmentElapsed() time.Duration {
	st.mu.Lock()
	defer st.mu.Unlock()
	return time.Since(st.segmentStartedAt)
}

// resumeAdmissionFailed returns ownership to the parked state when the caller
// canceled before the Agent runtime accepted the response. A concurrent turn
// cancellation changes the phase first and therefore remains authoritative.
func (st *turnState) resumeAdmissionFailed() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.phase != turnResuming {
		return false
	}
	st.setPhaseLocked(turnParked)
	return true
}

func (st *turnState) cancelRequested() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.phase == turnCancelDriven || st.phase == turnCancelIdle
}

func (st *turnState) beginTerminal() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.phase == turnTerminal || st.phase == turnReleased {
		return false
	}
	st.setPhaseLocked(turnTerminal)
	return true
}

func (st *turnState) terminalized() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.phase == turnTerminal || st.phase == turnReleased
}

func (st *turnState) released() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.phase == turnReleased
}

func (st *turnState) markReleased() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.setPhaseLocked(turnReleased)
}

func (st *turnState) lifecycleChange() <-chan struct{} {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.lifecycleChanged == nil {
		st.lifecycleChanged = make(chan struct{})
	}
	return st.lifecycleChanged
}

func (st *turnState) setPhaseLocked(next turnPhase) {
	if st.phase == next {
		return
	}
	st.phase = next
	st.signalLifecycleLocked()
}

func (st *turnState) signalLifecycleLocked() {
	if st.lifecycleChanged != nil {
		close(st.lifecycleChanged)
	}
	st.lifecycleChanged = make(chan struct{})
}

// appendSteering pushes one user message onto the pending-steering queue, or
// returns [ErrTurnNotFound] when the queue is already closed (the turn is
// terminating — see [turnState.flushed]) so the caller treats it like steering a
// turn that has ended.
func (st *turnState) appendSteering(message steeringMessage) error {
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
func (st *turnState) drainSteering() []steeringMessage {
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
func (st *turnState) closeAndDrainSteering() []steeringMessage {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.flushed = true
	out := st.steering
	st.steering = nil
	return out
}
