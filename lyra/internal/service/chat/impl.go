package chat

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// New returns the M1 [Service] implementation. The implementation
// is single-process — it holds in-memory state about live turns and
// fans events out to subscribers via per-turn channels.
//
// Future milestones extend this: session-store backing, multi-client
// event fan-out, plan-mode pause/resume, etc. The Service interface
// is stable, so transport adapters (M8+) don't care which impl they
// talk to.
func New(eng *engine.Engine) Service {
	if eng == nil {
		panic("chat: engine is required")
	}
	return &impl{engine: eng, turns: map[string]*turnState{}}
}

// turnState holds the per-turn bookkeeping the implementation needs:
// the event channel subscribers read from, the cancel func that
// fires when [Service.Cancel] is called, a monotone sequence number
// stamped onto every emitted event, and the plan-decision channel
// runTurn blocks on when the turn is in plan-pending state.
type turnState struct {
	handle TurnHandle
	events chan Event
	cancel context.CancelFunc
	seq    atomic.Uint64

	// planDecision is non-nil only while the turn is paused
	// waiting for [Service.ContinuePlan]. Buffered (cap 1) so a
	// ContinuePlan call never blocks regardless of runTurn's
	// progress. nil for non-plan-mode turns.
	planDecision chan PlanDecision
}

type impl struct {
	engine *engine.Engine

	mu    sync.Mutex
	turns map[string]*turnState // turn_id → state
}

// ------------------------------------------------------------------
// Service implementation
// ------------------------------------------------------------------

func (s *impl) StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error) {
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

func (s *impl) Events(_ context.Context, handle TurnHandle) (<-chan Event, error) {
	s.mu.Lock()
	state, ok := s.turns[handle.TurnID]
	s.mu.Unlock()
	if !ok {
		return nil, ErrTurnNotFound
	}
	return state.events, nil
}

func (s *impl) InjectSteering(_ context.Context, _ TurnHandle, _ string) error {
	// M1 leaves steering as a stub — surface stable so transport
	// adapters can call it; impl arrives with M3+ when multi-turn
	// + session persistence land.
	return errors.New("chat: steering not implemented in M1")
}

func (s *impl) Cancel(_ context.Context, handle TurnHandle) error {
	s.mu.Lock()
	state, ok := s.turns[handle.TurnID]
	s.mu.Unlock()
	if !ok {
		return ErrTurnNotFound
	}
	state.cancel()
	return nil
}

// ContinuePlan resumes a paused plan-mode turn. The decision is
// pushed to the buffered channel runTurn is waiting on; a closed
// channel (turn already over) returns [ErrTurnNotFound]. Plain
// chat turns (no PlanMode) have no planDecision channel and so
// also return ErrTurnNotFound — the wrong-mode case is folded in
// rather than introducing a separate sentinel.
func (s *impl) ContinuePlan(_ context.Context, handle TurnHandle, decision PlanDecision) error {
	s.mu.Lock()
	state, ok := s.turns[handle.TurnID]
	s.mu.Unlock()
	if !ok || state.planDecision == nil {
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

// ------------------------------------------------------------------
// Turn execution
// ------------------------------------------------------------------

// runTurn drives one turn from start to finish, emitting events as
// it goes. It always closes the event channel and clears the turn
// from the in-memory map so subsequent [Events] / [Cancel] return
// ErrTurnNotFound.
func (s *impl) runTurn(ctx context.Context, st *turnState, req StartTurnRequest) {
	defer func() {
		close(st.events)
		s.mu.Lock()
		delete(s.turns, st.handle.TurnID)
		s.mu.Unlock()
	}()

	startedAt := time.Now()
	s.emit(st, TurnStart{
		BaseEvent: st.baseEvent(),
		Model:     "default", // M1 — engine exposes model name in M2+
	})

	if req.PlanMode && !s.runPlanMode(ctx, st, req.Message, startedAt) {
		return
	}

	observer := &turnObserver{impl: s, st: st}
	_, runErr := s.engine.RunChat(ctx, engine.RunChatRequest{
		SessionID: req.SessionID,
		Message:   req.Message,
		Observer:  observer,
	})

	if runErr == nil && req.SessionID != "" {
		s.postTurnMaintenance(ctx, st, req.SessionID)
	}
	if runErr != nil {
		// Honour cancellation differently from genuine errors so
		// transport adapters can render the right state.
		if errors.Is(ctx.Err(), context.Canceled) {
			s.emit(st, TurnEnd{
				BaseEvent: st.baseEvent(),
				Reason:    TurnEndCancelled,
				Duration:  time.Since(startedAt),
			})
			return
		}
		s.emit(st, ErrorEvent{
			BaseEvent: st.baseEvent(),
			Message:   runErr.Error(),
			Code:      "ENGINE_ERROR",
		})
		s.emit(st, TurnEnd{
			BaseEvent: st.baseEvent(),
			Reason:    TurnEndErrored,
			Duration:  time.Since(startedAt),
		})
		return
	}

	// MessageDelta events already flowed through the observer during
	// the stream — no need to re-emit the assembled reply here.

	s.emit(st, TurnEnd{
		BaseEvent: st.baseEvent(),
		Reason:    TurnEndCompleted,
		Duration:  time.Since(startedAt),
		// TokenUsage / CostUSD wired up in M5 when invocation history
		// per-turn aggregation lands.
	})
}

// emit drops one event on the turn's channel. The send is non-blocking
// for cancellation safety — if the receiver has fallen behind we drop
// the event rather than block the turn forever.
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
func (s *impl) emit(st *turnState, ev Event) {
	stamped := ev.stamp(st.seq.Add(1), time.Now())
	select {
	case st.events <- stamped:
	default:
	}
}

func (st *turnState) baseEvent() BaseEvent {
	return BaseEvent{
		SessionID: st.handle.SessionID,
		TurnID:    st.handle.TurnID,
	}
}

// runPlanMode handles the plan-mode pre-flight: ask the LLM for a
// plan, emit it, then wait for the user's Approve / Reject.
// Returns true when execution should proceed (Approve, or NO_PLAN
// short-circuit). Returns false when the turn is over — the
// function has already emitted the appropriate TurnEnd /
// ErrorEvent before returning.
//
// Lives as a method so it shares the runTurn defer (cleanup +
// channel close) without duplicating it.
func (s *impl) runPlanMode(ctx context.Context, st *turnState, message string, startedAt time.Time) bool {
	plan, err := s.engine.GeneratePlan(ctx, message)
	if err != nil {
		s.emit(st, ErrorEvent{
			BaseEvent: st.baseEvent(),
			Message:   "plan generation failed: " + err.Error(),
			Code:      "PLANNING_ERROR",
		})
		s.emit(st, TurnEnd{
			BaseEvent: st.baseEvent(),
			Reason:    TurnEndErrored,
			Duration:  time.Since(startedAt),
		})
		return false
	}
	// Trivial requests (NO_PLAN → empty plan) skip approval and
	// fall through to direct execution.
	if plan == "" {
		return true
	}

	s.emit(st, PlanGenerated{
		BaseEvent: st.baseEvent(),
		Plan:      plan,
	})
	decision, ok := waitDecision(ctx, st)
	if !ok || decision == PlanReject {
		s.emit(st, TurnEnd{
			BaseEvent: st.baseEvent(),
			Reason:    TurnEndCancelled,
			Duration:  time.Since(startedAt),
		})
		return false
	}
	return true
}

// postTurnMaintenance runs the compact + (conditional) extract pair
// after the turn's real LLM round completed cleanly. Errors at
// this stage surface through ErrorEvent but don't abort the turn —
// the user's reply is already on screen.
//
// Fact extraction is gated on compaction firing: extraction is one
// extra LLM call, so we amortise it onto the moments where the
// runtime had to summarise anyway.
func (s *impl) postTurnMaintenance(ctx context.Context, st *turnState, sessionID string) {
	compacted, err := s.engine.MaybeCompact(ctx, sessionID)
	if err != nil {
		s.emit(st, ErrorEvent{
			BaseEvent: st.baseEvent(),
			Message:   "auto-compaction failed: " + err.Error(),
			Code:      "COMPACTION_ERROR",
		})
		return
	}
	if !compacted {
		return
	}
	if err := s.engine.MaybeExtract(ctx, sessionID); err != nil {
		s.emit(st, ErrorEvent{
			BaseEvent: st.baseEvent(),
			Message:   "memory extraction failed: " + err.Error(),
			Code:      "EXTRACTION_ERROR",
		})
	}
}

// waitDecision blocks until the client calls ContinuePlan or the
// turn context is cancelled. Returns the second value as false on
// cancellation so the caller emits TurnEndCancelled cleanly.
func waitDecision(ctx context.Context, st *turnState) (PlanDecision, bool) {
	select {
	case d := <-st.planDecision:
		return d, true
	case <-ctx.Done():
		return PlanReject, false
	}
}

// turnObserver bridges engine.ToolObserver to the turn's event
// channel. The engine fires Start / End for every tool the model
// invokes; we translate each into a Lyra ToolCallStart / ToolCallEnd
// event so transport adapters surface them verbatim.
type turnObserver struct {
	impl *impl
	st   *turnState
}

func (t *turnObserver) OnToolCallStart(callID, toolName, arguments string) {
	t.impl.emit(t.st, ToolCallStart{
		BaseEvent: t.st.baseEvent(),
		CallID:    callID,
		ToolName:  toolName,
		Arguments: arguments,
	})
}

func (t *turnObserver) OnToolCallEnd(callID, _ string, output string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	t.impl.emit(t.st, ToolCallEnd{
		BaseEvent: t.st.baseEvent(),
		CallID:    callID,
		Output:    output,
		Err:       errStr,
	})
}

func (t *turnObserver) OnMessageDelta(text string) {
	t.impl.emit(t.st, MessageDelta{
		BaseEvent: t.st.baseEvent(),
		Text:      text,
	})
}
