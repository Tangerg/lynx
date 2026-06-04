package chat

import (
	"context"
	"errors"
	"iter"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
)

// turnIDPrefix tags every turn id. A turn id doubles as the root run's
// wire id (runs.start returns it as runId), so it carries the run_ type
// prefix (API.md §2.2; mirrors protocol.IDPrefixRun).
const turnIDPrefix = "run_"

// newTurnID mints a fresh prefixed turn id.
func newTurnID() string { return turnIDPrefix + uuid.NewString() }

// New returns the [Service] implementation. The implementation is
// single-process — it holds in-memory state about live turns and
// fans events out to subscribers via per-turn channels.
//
// approvalSvc is optional. When non-nil the chat impl reads its mode
// at each tool call to decide run / deny / pause-for-approval; on nil
// every call passes (auto-approve, useful for tests / smoke runs).
//
// The implementation is split across files by concern:
//   - inmemory.go  — Service surface + live-turn registry (this file)
//   - turn.go      — per-turn state + the runTurn execution loop
//   - lifecycle.go — terminal-event capture from the agent runtime
//   - observer.go  — engine.ToolObserver → chat.Event translation
//
// The Service interface is stable, so transport adapters don't care
// which impl they talk to.
// resolver is optional. When non-nil and a turn carries a Model, the impl
// resolves a per-turn client for that model; nil (or an empty Model) runs
// every turn on the platform's default client.
func New(eng Engine, approvalSvc approval.Service, resolver ClientResolver) Service {
	if eng == nil {
		panic("chat: engine is required")
	}
	return &inMemory{
		engine:   eng,
		approval: approvalSvc,
		resolver: resolver,
		turns:    map[string]*turnState{},
	}
}

// inMemory is the single-process [Service] implementation. It
// tracks live turns in a map keyed by turn id; state lives in
// process memory and does not survive restart.
type inMemory struct {
	engine   Engine
	approval approval.Service // optional — nil = auto-approve every tool
	resolver ClientResolver   // optional — nil = always use the default model

	mu    sync.Mutex
	turns map[string]*turnState // turn_id → state

	// interruptKinds is the allowlist of HITL kinds the connected client
	// declared it can answer (ClientCapabilities.InterruptKinds). nil means
	// unconfigured → surface every kind (the permissive default for
	// in-process / CLI callers that don't negotiate). A non-nil set gates
	// strictly: a turn about to park on a kind absent here is auto-denied
	// rather than left as an unanswerable interrupt (API.md §6.2). Guarded
	// by mu.
	interruptKinds map[string]bool
}

// ------------------------------------------------------------------
// Service implementation
// ------------------------------------------------------------------

func (s *inMemory) StartTurn(_ context.Context, req StartTurnRequest) (TurnHandle, error) {
	if req.SessionID == "" {
		return TurnHandle{}, errors.New("chat: SessionID is required")
	}
	if req.Message == "" {
		return TurnHandle{}, errors.New("chat: Message must not be empty")
	}

	handle := TurnHandle{
		SessionID: req.SessionID,
		TurnID:    newTurnID(),
	}
	state := newTurnState(handle)

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	go s.runTurn(req, state)

	return handle, nil
}

// newTurnState builds a fresh per-turn state. Cancellation derives from a
// background ctx so the caller's ctx ending (e.g. the StartTurn RPC
// returning) doesn't kill the in-flight turn; the same ctx bounds the run
// and any resume continuation. Shared by StartTurn and Rehydrate so both
// entry points produce an identically-initialized turn.
func newTurnState(handle TurnHandle) *turnState {
	ctx, cancel := context.WithCancel(context.Background())
	return &turnState{
		handle:    handle,
		events:    make(chan Event, 32),
		cancel:    cancel,
		ctx:       ctx,
		startedAt: time.Now(),
	}
}

// findTurn looks up the per-turn state by id under the impl's
// mutex. Returns ErrTurnNotFound when the turn has already ended
// (runTurn deletes itself from the map on exit). Centralizes the
// "lock / lookup / unlock" sequence every public method below
// needs to perform.
func (s *inMemory) findTurn(id string) (*turnState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.turns[id]
	if !ok {
		return nil, ErrTurnNotFound
	}
	return state, nil
}

func (s *inMemory) Events(ctx context.Context, handle TurnHandle) (iter.Seq[Event], error) {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return nil, err
	}
	// Single-consumer pull stream. The internal select multiplexes the
	// turn's event channel against ctx so the iterator stops promptly
	// when the caller stops listening — even while parked waiting for
	// the next event. runTurn closes state.events on turn end, which
	// terminates the range cleanly (ok == false).
	return func(yield func(Event) bool) {
		for {
			select {
			case ev, ok := <-state.events:
				if !ok || !yield(ev) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

// InjectSteering queues message onto the active turn's pending
// steering buffer. The runtime flushes the queue to the chat-memory
// store after the turn ends — every queued message becomes a
// user-role entry in the conversation history that the next turn's
// chat-memory middleware loads on the next StartTurn.
//
// This is "next-turn" semantics — not true mid-stream injection.
// Steering you send while the model is mid-tool-loop affects the
// next turn, not the current one. Documented limitation; doing
// real mid-stream injection would require intercepting between
// rounds of the chat tool middleware.
//
// Returns [ErrTurnNotFound] when the turn has already ended (its
// runTurn deleted itself from the map on exit). Empty messages
// are silently dropped.
func (s *inMemory) InjectSteering(_ context.Context, handle TurnHandle, message string) error {
	if message == "" {
		return nil
	}
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	state.appendSteering(message)
	return nil
}

// Cancel routes through the agent runtime when the chat process has
// already dispatched — Platform.KillProcess flips the process to
// StatusKilled and the run loop exits at its next checkpoint. The
// ctx cancel still fires too so any in-flight LLM stream (which
// reads ctx.Done()) aborts promptly. For turns still in plan-mode
// (proc not yet populated), only the ctx cancel applies — runTurn
// observes ctx.Done() during waitDecision and emits TurnEndCancelled.
func (s *inMemory) Cancel(_ context.Context, handle TurnHandle) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	state.cancel()
	// proc/parked are written by runTurn/drive on other goroutines; read
	// under the lock. Claim the parked flag so a racing Resume can't also
	// act on the same suspended turn (whoever flips it false wins).
	s.mu.Lock()
	proc := state.proc
	claimed := state.parked
	state.parked = false
	s.mu.Unlock()
	if proc != nil {
		_ = proc.Cancel("user cancel")
	}
	if claimed {
		// The turn was parked on an interrupt — no drive goroutine is
		// waiting on it, so emit the terminal + tear down here.
		s.finishTurn(state, TurnEndCancelled)
	}
	return nil
}

// Resume answers a turn parked on a HITL interrupt (tool approval or
// plan review). It claims the parked flag (so a racing Cancel can't
// double-act), delivers the bool decision to the agent process, and
// drives the continuation segment onto the same event channel. Returns
// [ErrTurnNotFound] when the turn isn't parked (unknown / already
// resumed / terminal).
func (s *inMemory) Resume(_ context.Context, handle TurnHandle, approved bool) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if !state.parked {
		s.mu.Unlock()
		return ErrTurnNotFound
	}
	state.parked = false
	s.mu.Unlock()

	return s.resumeAndDrive(state, approved)
}

// resumeAndDrive delivers the decision to the turn's (write-once-stable)
// parked process and launches the continuation drive. On a resume error it
// streams the terminal (ErrorEvent + TurnEnd) and returns the error;
// otherwise it starts drive and returns nil. Shared by [Resume]
// (same-process) and [Rehydrate] (cross-restart) so the resume tail —
// deliver, on-error-finish, else-drive — stays identical.
func (s *inMemory) resumeAndDrive(state *turnState, approved bool) error {
	resumed, err := state.proc.Resume(state.ctx, approved)
	if err != nil {
		s.emit(state, ErrorEvent{Message: err.Error(), Code: "ENGINE_ERROR"})
		s.finishTurn(state, TurnEndErrored)
		return err
	}
	go s.drive(state, resumed)
	return nil
}

// SetInterruptKinds records the HITL kinds the connected client can
// answer (from ClientCapabilities.InterruptKinds, negotiated at
// runtime.initialize). Passing an empty slice gates every kind; never
// calling it leaves the permissive default (surface all). Single-tenant:
// one client's negotiation applies process-wide.
func (s *inMemory) SetInterruptKinds(kinds []string) {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	s.mu.Lock()
	s.interruptKinds = set
	s.mu.Unlock()
}

// canSurface reports whether a turn may park on an interrupt of kind —
// true when no allowlist is configured (permissive default) or kind is in
// it. A false result means the client can't answer this kind, so the turn
// auto-denies instead of deadlocking.
func (s *inMemory) canSurface(kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interruptKinds == nil {
		return true
	}
	return s.interruptKinds[kind]
}

// ProcessID returns the agent-process id backing a live turn — the
// snapshot key the runtime persists so a restart can rebuild the process
// via [Rehydrate]. Returns [ErrTurnNotFound] when the turn isn't live.
func (s *inMemory) ProcessID(_ context.Context, handle TurnHandle) (string, error) {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	proc := state.proc
	s.mu.Unlock()
	if proc == nil {
		return "", errors.New("chat: turn has not dispatched a process yet")
	}
	return proc.ID(), nil
}

// Rehydrate rebuilds a turn from a persisted process snapshot and resumes
// it — the cross-restart counterpart to [Resume]. It registers a fresh
// turn (new handle), restores + re-parks the agent process via
// [engine.RestoreChat] with a fresh observer + lifecycle listener, then
// delivers the decision and drives the continuation onto the new turn's
// event channel. The caller subscribes via [Events] on the returned handle.
func (s *inMemory) Rehydrate(_ context.Context, req RehydrateRequest) (TurnHandle, error) {
	if req.ProcessID == "" {
		return TurnHandle{}, errors.New("chat: ProcessID is required")
	}
	handle := TurnHandle{SessionID: req.SessionID, TurnID: newTurnID()}
	state := newTurnState(handle)
	observer := &turnObserver{svc: s, st: state}
	state.lifecycle = &turnLifecycle{}

	proc, err := s.engine.RestoreChat(state.ctx, req.ProcessID, engine.RestoreChatRequest{
		SessionID:     req.SessionID,
		Observer:      observer,
		EventListener: state.lifecycle.listener(handle.TurnID),
	})
	if err != nil {
		state.cancel()
		return TurnHandle{}, err
	}
	state.lifecycle.setRoot(proc.ID())
	state.proc = proc

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	// The restored process is re-parked (RestoreChat re-ticked it). Deliver
	// the decision and drive the continuation; on a resume error the
	// terminal is already streamed, so the handle is still returned for the
	// caller to drain.
	_ = s.resumeAndDrive(state, req.Approved)
	return handle, nil
}

// emit stamps the event with the next sequence number and timestamp
// and pushes it onto the turn's channel. Type-specific stamping
// lives on each concrete event (via the unexported [Event.stamp]
// method) so this dispatcher stays open-closed — adding a new
// event variant means writing the struct + one stamp method,
// nothing here.
//
// Sends are non-blocking: if the receiver has fallen behind, we
// drop the event rather than stall the turn. A future enhancement
// could buffer dropped events into an outbox + metric counter so
// slow clients are visible in observability.
func (s *inMemory) emit(st *turnState, ev Event) {
	stamped := ev.stamp(BaseEvent{
		SessionID: st.handle.SessionID,
		TurnID:    st.handle.TurnID,
		Seq:       st.seq.Add(1),
		Timestamp: time.Now(),
	})
	select {
	case st.events <- stamped:
	default:
	}
}
