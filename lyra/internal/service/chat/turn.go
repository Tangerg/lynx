package chat

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// turnState holds the per-turn bookkeeping the implementation needs:
// the event channel subscribers read from, the cancel func that
// fires when [Service.Cancel] is called, a monotone sequence number
// stamped onto every emitted event, and the plan-decision channel
// runTurn blocks on when the turn is in plan-pending state.
//
// Once the chat agent dispatches, proc holds the running
// [engine.ChatProcess]; [Service.Cancel] routes through it so the
// agent runtime (not just ctx cancellation) drives termination.
type turnState struct {
	handle TurnHandle
	events chan Event
	cancel context.CancelFunc
	seq    atomic.Uint64

	// proc is the agent process backing this turn. Populated once
	// runTurn calls engine.StartChat (after plan approval in
	// plan-mode), nil before that. Cancel and Status both inspect
	// it.
	proc engine.ChatProcess

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
	lifecycle := &turnLifecycle{}
	proc := s.engine.StartChat(ctx, engine.RunChatRequest{
		SessionID:     req.SessionID,
		Message:       req.Message,
		MaxBudget:     req.MaxBudget,
		MaxCostUSD:    req.MaxCostUSD,
		Observer:      observer,
		EventListener: lifecycle.listener(st.handle.TurnID),
	})
	st.proc = proc
	runErr := <-proc.Done()

	// Drain any steering the client injected during the turn so it
	// lands in conversation history BEFORE post-turn maintenance —
	// the compactor / extractor then see steering as part of the
	// conversation they summarize.
	s.flushSteering(ctx, st, req.SessionID)

	if runErr == nil && req.SessionID != "" {
		s.postTurnMaintenance(ctx, st, req.SessionID)
	}

	// MessageDelta events already flowed through the observer during
	// the stream — no need to re-emit the assembled reply here.
	s.emitTurnEnd(st, proc, lifecycle.get(), runErr, time.Since(startedAt), ctx.Err())
}

// emitTurnEnd maps the captured agent runtime terminal event onto a
// transport-shape TurnEnd. The lifecycle listener fires terminal
// events authoritatively (ProcessKilled / ProcessFailed /
// ProcessStuck / ProcessTerminated / ProcessCompleted), so we read
// those rather than re-deriving status from the run loop's error.
// The runErr / ctxErr / status fallback covers stub tests where no
// listener fired and any race where Done() returned before the
// platform multicast delivered the terminal event.
func (s *inMemory) emitTurnEnd(st *turnState, proc engine.ChatProcess, terminal event.Event, runErr error, duration time.Duration, ctxErr error) {
	switch ev := terminal.(type) {
	case event.ProcessCompleted:
		_ = ev
		out, _ := proc.Output()
		reason := TurnEndCompleted
		if out.StoppedOnBudget {
			// The action returned normally (no error) but stopped early
			// on the token ceiling — surface that as the terminal reason
			// rather than a plain completion.
			reason = TurnEndBudgetExceeded
		}
		s.emit(st, TurnEnd{Reason: reason, Duration: duration, TokenUsage: out.Usage, UsageByModel: out.UsageByModel, CostUSD: out.CostUSD})
	case event.ProcessKilled, event.ProcessTerminated:
		_ = ev
		s.emit(st, TurnEnd{Reason: TurnEndCancelled, Duration: duration})
	case event.ProcessFailed:
		msg := "engine error"
		if ev.Err != nil {
			msg = ev.Err.Error()
		}
		s.emit(st, ErrorEvent{Message: msg, Code: "ENGINE_ERROR"})
		s.emit(st, TurnEnd{Reason: TurnEndErrored, Duration: duration})
	case event.ProcessStuck:
		_ = ev
		s.emit(st, ErrorEvent{Message: "agent stuck — planner produced no plan", Code: "AGENT_STUCK"})
		s.emit(st, TurnEnd{Reason: TurnEndErrored, Duration: duration})
	default:
		// Listener didn't capture a terminal event — stub tests or
		// pre-platform race. Fall back to the run-loop signals.
		if runErr != nil {
			if proc.Status() == core.StatusKilled || errors.Is(ctxErr, context.Canceled) {
				s.emit(st, TurnEnd{Reason: TurnEndCancelled, Duration: duration})
				return
			}
			s.emit(st, ErrorEvent{Message: runErr.Error(), Code: "ENGINE_ERROR"})
			s.emit(st, TurnEnd{Reason: TurnEndErrored, Duration: duration})
			return
		}
		out, _ := proc.Output()
		reason := TurnEndCompleted
		if out.StoppedOnBudget {
			reason = TurnEndBudgetExceeded
		}
		s.emit(st, TurnEnd{Reason: reason, Duration: duration, TokenUsage: out.Usage, UsageByModel: out.UsageByModel, CostUSD: out.CostUSD})
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
// Both maintenance actions are observable: a fired compaction emits
// [CompactBoundary] (before/after message counts) and a successful
// extraction emits [MemoryUpdated] (the facts saved). Surfacing them
// keeps the runtime's housekeeping visible to clients instead of
// silently mutating context behind the user's back — the SDK's
// SDKCompactBoundaryMessage / memory-event spirit, adapted.
//
// Fact extraction is gated on compaction firing: extraction is one
// extra LLM call, so we amortize it onto the moments where the
// runtime had to summarize anyway.
func (s *inMemory) postTurnMaintenance(ctx context.Context, st *turnState, sessionID string) {
	compaction, err := s.engine.MaybeCompact(ctx, sessionID)
	if err != nil {
		s.emit(st, ErrorEvent{
			Message: "auto-compaction failed: " + err.Error(),
			Code:    "COMPACTION_ERROR",
		})
		return
	}
	if !compaction.Compacted {
		return
	}
	s.emit(st, CompactBoundary{
		MessagesBefore: compaction.MessagesBefore,
		MessagesAfter:  compaction.MessagesAfter,
	})

	extraction, err := s.engine.MaybeExtract(ctx, sessionID)
	if err != nil {
		s.emit(st, ErrorEvent{
			Message: "memory extraction failed: " + err.Error(),
			Code:    "EXTRACTION_ERROR",
		})
		return
	}
	if extraction.Extracted {
		s.emit(st, MemoryUpdated{Facts: extraction.Facts})
	}
}
