package chat

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/engine"
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
// Future milestones extend this: session-store backing,
// multi-client event fan-out, plan-mode pause/resume, etc. The
// Service interface is stable, so transport adapters don't care
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

	// steerMu guards steering — the queue of mid-turn user
	// messages injected via [Service.InjectSteering]. The runtime
	// flushes the queue to the chat-memory store after the turn
	// ends so the messages land in conversation history for the
	// next turn.
	steerMu  sync.Mutex
	steering []string
}

// appendSteering atomically pushes one user message onto the
// turn's pending-steering queue.
func (st *turnState) appendSteering(message string) {
	st.steerMu.Lock()
	defer st.steerMu.Unlock()
	st.steering = append(st.steering, message)
}

// drainSteering atomically returns the queued steering messages
// and clears the queue. Returns nil when no steering is pending.
func (st *turnState) drainSteering() []string {
	st.steerMu.Lock()
	defer st.steerMu.Unlock()
	if len(st.steering) == 0 {
		return nil
	}
	out := st.steering
	st.steering = nil
	return out
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

func (s *inMemory) StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error) {
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

func (s *inMemory) Cancel(_ context.Context, handle TurnHandle) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
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

// ------------------------------------------------------------------
// Turn execution
// ------------------------------------------------------------------

// runTurn drives one turn from start to finish, emitting events as
// it goes. It always closes the event channel and clears the turn
// from the in-memory map so subsequent [Events] / [Cancel] return
// ErrTurnNotFound.
func (s *inMemory) runTurn(ctx context.Context, st *turnState, req StartTurnRequest) {
	defer func() {
		close(st.events)
		s.mu.Lock()
		delete(s.turns, st.handle.TurnID)
		s.mu.Unlock()
	}()

	startedAt := time.Now()
	s.emit(st, TurnStart{
		Model: "default", // M1 — engine exposes model name in M2+
	})

	if req.PlanMode && !s.runPlanMode(ctx, st, req.Message, startedAt) {
		return
	}

	observer := &turnObserver{svc: s, st: st}
	out, runErr := s.engine.RunChat(ctx, engine.RunChatRequest{
		SessionID: req.SessionID,
		Message:   req.Message,
		Observer:  observer,
	})

	// Drain any steering the client injected during the turn so it
	// lands in conversation history BEFORE post-turn maintenance —
	// the compactor / extractor then see steering as part of the
	// conversation they summarize.
	s.flushSteering(ctx, st, req.SessionID)

	if runErr == nil && req.SessionID != "" {
		s.postTurnMaintenance(ctx, st, req.SessionID)
	}
	if runErr != nil {
		// Honor cancellation differently from genuine errors so
		// transport adapters can render the right state.
		if errors.Is(ctx.Err(), context.Canceled) {
			s.emit(st, TurnEnd{
				Reason:   TurnEndCancelled,
				Duration: time.Since(startedAt),
			})
			return
		}
		s.emit(st, ErrorEvent{
			Message: runErr.Error(),
			Code:    "ENGINE_ERROR",
		})
		s.emit(st, TurnEnd{
			Reason:   TurnEndErrored,
			Duration: time.Since(startedAt),
		})
		return
	}

	// MessageDelta events already flowed through the observer during
	// the stream — no need to re-emit the assembled reply here.

	s.emit(st, TurnEnd{
		Reason:     TurnEndCompleted,
		Duration:   time.Since(startedAt),
		TokenUsage: out.Usage,
		// CostUSD requires per-provider pricing — see M-future.
	})
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

// runPlanMode handles the plan-mode pre-flight: ask the LLM for a
// plan, emit it, then wait for the user's Approve / Reject.
// Returns true when execution should proceed (Approve, or NO_PLAN
// short-circuit). Returns false when the turn is over — the
// function has already emitted the appropriate TurnEnd /
// ErrorEvent before returning.
//
// Lives as a method so it shares the runTurn defer (cleanup +
// channel close) without duplicating it.
func (s *inMemory) runPlanMode(ctx context.Context, st *turnState, message string, startedAt time.Time) bool {
	plan, err := s.engine.GeneratePlan(ctx, message)
	if err != nil {
		s.emit(st, ErrorEvent{
			Message: "plan generation failed: " + err.Error(),
			Code:    "PLANNING_ERROR",
		})
		s.emit(st, TurnEnd{
			Reason:   TurnEndErrored,
			Duration: time.Since(startedAt),
		})
		return false
	}
	// Trivial requests (NO_PLAN → empty plan) skip approval and
	// fall through to direct execution.
	if plan == "" {
		return true
	}

	s.emit(st, PlanGenerated{
		Plan: plan,
	})
	decision, ok := st.waitDecision(ctx)
	if !ok || decision == PlanReject {
		s.emit(st, TurnEnd{
			Reason:   TurnEndCancelled,
			Duration: time.Since(startedAt),
		})
		return false
	}
	return true
}

// flushSteering writes the turn's queued steering messages to the
// chat-memory store so the next turn picks them up as conversation
// history. No-op when there's no session or no queued steering.
// Errors surface through an ErrorEvent but don't abort the turn —
// dropping steering is preferable to wrecking an otherwise
// successful turn.
func (s *inMemory) flushSteering(ctx context.Context, st *turnState, sessionID string) {
	queue := st.drainSteering()
	if sessionID == "" || len(queue) == 0 {
		return
	}
	for _, msg := range queue {
		if err := s.engine.InjectUserMessage(ctx, sessionID, msg); err != nil {
			s.emit(st, ErrorEvent{
				Message: "steering inject failed: " + err.Error(),
				Code:    "STEERING_ERROR",
			})
			return
		}
	}
}

// postTurnMaintenance runs the compact + (conditional) extract pair
// after the turn's real LLM round completed cleanly. Errors at
// this stage surface through ErrorEvent but don't abort the turn —
// the user's reply is already on screen.
//
// Fact extraction is gated on compaction firing: extraction is one
// extra LLM call, so we amortize it onto the moments where the
// runtime had to summarize anyway.
func (s *inMemory) postTurnMaintenance(ctx context.Context, st *turnState, sessionID string) {
	compacted, err := s.engine.MaybeCompact(ctx, sessionID)
	if err != nil {
		s.emit(st, ErrorEvent{
			Message: "auto-compaction failed: " + err.Error(),
			Code:    "COMPACTION_ERROR",
		})
		return
	}
	if !compacted {
		return
	}
	if err := s.engine.MaybeExtract(ctx, sessionID); err != nil {
		s.emit(st, ErrorEvent{
			Message: "memory extraction failed: " + err.Error(),
			Code:    "EXTRACTION_ERROR",
		})
	}
}

// waitDecision blocks until the client calls ContinuePlan or the
// turn context is canceled. Returns the second value as false on
// cancellation so the caller emits TurnEndCancelled cleanly.
//
// Lives on *turnState (not as a free function) because the state
// owns the planDecision channel — keeping the method here matches
// the rest of the file's "behavior lives on the type that holds
// the data" convention.
func (st *turnState) waitDecision(ctx context.Context) (PlanDecision, bool) {
	select {
	case d := <-st.planDecision:
		return d, true
	case <-ctx.Done():
		return PlanReject, false
	}
}

// turnObserver bridges engine.ToolObserver to the turn's event
// channel. The engine fires Approve / Start / End for every tool
// the model invokes; we translate each into a Lyra ToolCall*
// event so transport adapters surface them verbatim.
type turnObserver struct {
	svc *inMemory
	st  *turnState
}

// OnToolCallApprove is the gate the engine fires BEFORE every tool
// call. When the configured [approval.Service] mode + the tool's
// safety class agree to auto-pass the call, the gate returns nil
// immediately and the tool runs. Otherwise the gate registers the
// pending request, emits a [ToolCallApproval] event onto the turn
// channel, and blocks on the decision channel until the client
// posts a verdict via [approval.Service.Decide].
//
// Returns nil to proceed, an error to short-circuit. The engine
// surfaces the error back to the model as the tool's "output"
// (engine.observedTool collapses Deny into a non-fatal tool
// result) so the model can recover without aborting the turn.
func (t *turnObserver) OnToolCallApprove(ctx context.Context, callID, toolName, arguments string) error {
	if t.svc.approval == nil {
		return nil
	}
	mode, err := t.svc.approval.GetMode(ctx)
	if err != nil {
		return err
	}
	if !needsApproval(toolName, mode) {
		return nil
	}

	req := approval.Request{
		ID:          callID,
		SessionID:   t.st.handle.SessionID,
		TurnID:      t.st.handle.TurnID,
		ToolName:    toolName,
		Arguments:   arguments,
		RequestedAt: time.Now(),
	}
	// Register BEFORE emit so a Decide that arrives the instant
	// the client sees the event has a pending entry to resolve.
	decisionCh, cleanup := t.svc.approval.Register(req)
	defer cleanup()

	t.svc.emit(t.st, ToolCallApproval{Request: req})

	select {
	case d := <-decisionCh:
		if d == approval.DecisionDeny {
			return errors.New("tool call denied by user")
		}
		return nil
	case <-ctx.Done():
		// Fail-closed on turn cancellation.
		return ctx.Err()
	}
}

func (t *turnObserver) OnToolCallStart(callID, toolName, arguments string) {
	t.svc.emit(t.st, ToolCallStart{
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
	t.svc.emit(t.st, ToolCallEnd{
		CallID: callID,
		Output: output,
		Err:    errStr,
	})
}

func (t *turnObserver) OnMessageDelta(text string) {
	t.svc.emit(t.st, MessageDelta{
		Text: text,
	})
}

// OnReasoningDelta forwards extended-thinking chunks to the turn
// channel as [ReasoningDelta] events. Clients that don't care
// about reasoning can ignore the type in their dispatch switch —
// no event is dropped on the engine side.
func (t *turnObserver) OnReasoningDelta(text string) {
	t.svc.emit(t.st, ReasoningDelta{
		Text: text,
	})
}
