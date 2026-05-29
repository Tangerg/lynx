package chat

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
)

// New returns the [Service] implementation. The implementation is
// single-process — it holds in-memory state about live turns and
// fans events out to subscribers via per-turn channels.
//
// approvalSvc is optional. When non-nil the chat impl threads
// every tool call through it for permission gating; on nil the
// gate is a no-op and every call passes (legacy YOLO behavior
// useful for tests / smoke runs).
//
// The implementation is split across files by concern:
//   - inmemory.go  — Service surface + live-turn registry (this file)
//   - turn.go      — per-turn state + the runTurn execution loop
//   - lifecycle.go — terminal-event capture from the agent runtime
//   - observer.go  — engine.ToolObserver → chat.Event translation
//
// The Service interface is stable, so transport adapters don't care
// which impl they talk to.
func New(eng Engine, approvalGate approval.Gate) Service {
	if eng == nil {
		panic("chat: engine is required")
	}
	return &inMemory{
		engine:   eng,
		approval: approvalGate,
		turns:    map[string]*turnState{},
	}
}

// inMemory is the single-process [Service] implementation. It
// tracks live turns in a map keyed by turn id; state lives in
// process memory and does not survive restart.
type inMemory struct {
	engine   Engine
	approval approval.Gate // optional — nil = auto-approve every tool

	mu    sync.Mutex
	turns map[string]*turnState // turn_id → state
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
		TurnID:    uuid.NewString(),
	}

	// Cancellation is per-turn — derive from a background ctx so the
	// caller's ctx ending (e.g. the StartTurn RPC returning) doesn't
	// kill the in-flight turn.
	turnCtx, cancel := context.WithCancel(context.Background())

	state := &turnState{
		handle: handle,
		events: make(chan Event, 32),
		cancel: cancel,
	}
	if req.PlanMode {
		state.planDecision = make(chan PlanDecision, 1)
	}

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	go s.runTurn(turnCtx, state, req)

	return handle, nil
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

func (s *inMemory) Events(_ context.Context, handle TurnHandle) (<-chan Event, error) {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return nil, err
	}
	return state.events, nil
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
	if state.proc != nil {
		// Cancel returns an error only on unknown id, which can't
		// happen here — proc is the live process. Ignore.
		_ = state.proc.Cancel("user cancel")
	}
	return nil
}

// ContinuePlan resumes a paused plan-mode turn. The decision is
// pushed to the buffered channel runTurn is waiting on; a closed
// channel (turn already over) returns [ErrTurnNotFound]. Plain
// chat turns (no PlanMode) have no planDecision channel and so
// also return ErrTurnNotFound — the wrong-mode case is folded in
// rather than introducing a separate sentinel.
func (s *inMemory) ContinuePlan(_ context.Context, handle TurnHandle, decision PlanDecision) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	if state.planDecision == nil {
		return ErrTurnNotFound
	}
	select {
	case state.planDecision <- decision:
		return nil
	default:
		// Decision already received — second ContinuePlan call is a no-op
		// rather than an error so transport retries are safe.
		return nil
	}
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
