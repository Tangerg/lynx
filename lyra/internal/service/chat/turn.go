package chat

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	corechat "github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// turnState holds the per-turn bookkeeping the implementation needs:
// the event channel subscribers read from, the cancel func that fires
// when [Service.Cancel] is called, a monotone sequence number stamped
// onto every emitted event, and the parked flag that marks a turn
// suspended on a HITL interrupt awaiting [Service.Resume].
//
// Once the chat agent dispatches, proc holds the running
// [engine.ChatProcess]; [Service.Cancel] routes through it so the
// agent runtime (not just ctx cancellation) drives termination.
type turnState struct {
	handle TurnHandle
	events chan Event
	cancel context.CancelFunc
	seq    atomic.Uint64

	// ctx is the turn's own lifetime context (background-derived so it
	// outlives the StartTurn caller's ctx). It bounds the run, the
	// resume continuation, and post-turn maintenance; canceled by
	// [Service.Cancel]. Set once in StartTurn.
	ctx context.Context

	// startedAt stamps the turn's wall-clock start so TurnEnd carries a
	// duration that spans any interrupt/resume cycles.
	startedAt time.Time

	// proc is the agent process backing this turn. runTurn writes it once
	// (under the impl mutex); [Service.Cancel] / [Service.Resume] read it
	// under the same mutex from other goroutines. The value is stable
	// after that single write, so callers may invoke proc's methods after
	// releasing the lock.
	proc engine.ChatProcess

	// lifecycle captures the process's authoritative terminal event;
	// retained across interrupt→resume so the eventual TurnEnd reads it.
	lifecycle *turnLifecycle

	// parked is true while the turn is suspended on a HITL interrupt
	// (StatusWaiting) awaiting [Service.Resume]. Guarded by the impl
	// mutex. A parked turn stays registered (events channel open) until
	// resumed to a terminal state or canceled.
	parked bool

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

// runTurn starts the turn's agent process and drives its first run
// segment to a suspension point — a HITL interrupt (park) or a terminal
// state. Later segments are driven by [inMemory.Resume] through the
// shared [drive] loop. st.ctx (the turn's own lifetime) bounds the run.
func (s *inMemory) runTurn(req StartTurnRequest, st *turnState) {
	model := req.Model
	if model == "" {
		model = "default"
	}
	s.emit(st, TurnStart{Model: model})

	// Resolve a per-turn client when the run picked a provider+model and a
	// resolver is wired; no selection / no resolver runs on the platform's
	// default client.
	var client *corechat.Client
	if req.Provider != "" && req.Model != "" && s.resolver != nil {
		c, err := s.resolver.ResolveClient(st.ctx, req.Provider, req.Model)
		if err != nil {
			s.emit(st, ErrorEvent{Message: err.Error(), Code: "MODEL_UNAVAILABLE"})
			s.finishTurn(st, TurnEndErrored)
			return
		}
		client = c
	}

	observer := &turnObserver{svc: s, st: st}
	st.lifecycle = &turnLifecycle{}
	proc := s.engine.StartChat(st.ctx, engine.RunChatRequest{
		SessionID:     req.SessionID,
		Message:       req.Message,
		Cwd:           req.Cwd,
		MaxBudget:     req.MaxBudget,
		MaxCostUSD:    req.MaxCostUSD,
		PlanMode:      req.PlanMode,
		ChatClient:    client,
		Observer:      observer,
		EventListener: st.lifecycle.listener(st.handle.TurnID),
	})
	// Record the root process id so the lifecycle gate keeps subtask
	// terminals (which fire first) from being mistaken for the turn's end.
	st.lifecycle.setRoot(proc.ID())
	// Guarded by s.mu: Cancel / Resume read st.proc from other goroutines.
	s.mu.Lock()
	st.proc = proc
	s.mu.Unlock()

	s.drive(st, proc.Done())
}

// drive consumes one run segment's completion. When the process parks
// on a HITL interrupt (StatusWaiting) it surfaces a [TurnInterrupted]
// and leaves the turn registered (events channel open) for
// [inMemory.Resume]. On a terminal state it drains steering, runs
// post-turn maintenance on a clean finish, emits [TurnEnd], and tears
// the turn down. doneCh is the segment's Done channel — the process's
// for the first segment, the resume continuation's thereafter.
func (s *inMemory) drive(st *turnState, doneCh <-chan error) {
	runErr := <-doneCh
	s.mu.Lock()
	proc := st.proc
	s.mu.Unlock()

	if proc.Status() == core.StatusWaiting {
		s.handleWaiting(st, proc)
		return
	}

	// Drain steering into history BEFORE maintenance so the compactor /
	// extractor see it as part of the conversation they summarize.
	s.flushSteering(st.ctx, st, st.handle.SessionID)
	if runErr == nil && st.handle.SessionID != "" {
		s.postTurnMaintenance(st.ctx, st, st.handle.SessionID)
	}
	// MessageDelta events already streamed through the observer — no
	// need to re-emit the assembled reply here.
	s.emitTurnEnd(st, proc, st.lifecycle.get(), runErr, time.Since(st.startedAt), st.ctx.Err())
	s.endTurn(st)
}

// handleWaiting decides what to do when the process parks at
// StatusWaiting. If the pending interrupt's kind is one the client can
// answer (see [inMemory.canSurface]) it surfaces it via
// [inMemory.emitInterrupt] and the turn waits for [inMemory.Resume].
// Otherwise the client could never answer it, so rather than leave a
// deadlocked interrupt (API.md §6.2) the turn auto-denies (via the shared
// [inMemory.resumeAndDrive]) and the continuation runs to a real terminal.
func (s *inMemory) handleWaiting(st *turnState, proc engine.ChatProcess) {
	aw := proc.PendingAwaitable()
	if aw == nil || s.canSurface(interruptKind(aw)) {
		s.emitInterrupt(st, proc)
		return
	}
	// Client can't answer this kind — deliver a deny and drive the
	// continuation (resumeAndDrive streams the terminal on a resume error
	// and launches drive otherwise; the returned error is already surfaced
	// on the channel, so it's safe to drop here).
	_ = s.resumeAndDrive(st, false)
}

// emitInterrupt marks the turn parked and surfaces the pending HITL
// request as a [TurnInterrupted] event. The turn stays registered with
// its events channel open; [inMemory.Resume] drives the next segment.
func (s *inMemory) emitInterrupt(st *turnState, proc engine.ChatProcess) {
	aw := proc.PendingAwaitable()
	s.mu.Lock()
	st.parked = true
	s.mu.Unlock()
	if aw == nil {
		// Defensive: Waiting without a parked awaitable shouldn't happen;
		// surface an empty interrupt rather than silently dropping it.
		s.emit(st, TurnInterrupted{})
		return
	}
	s.emit(st, TurnInterrupted{Interrupts: []Interrupt{{Kind: interruptKind(aw), Payload: aw.PromptAny()}}})
}

// interruptKind classifies the pending awaitable into the wire interrupt
// kind (API.md §6: "approval" | "question" | "toolResult"). An
// [ApprovalPrompt] payload is a gated tool call ("approval"); anything
// else is a plan awaiting review, which surfaces as a "question" (the
// contract has no "plan" interrupt kind — plan-review uses the generic
// question mechanism). Returns "" for a nil awaitable (treated as
// surfaceable so the defensive empty-interrupt path in emitInterrupt
// still fires).
func interruptKind(aw core.Awaitable) string {
	if aw == nil {
		return ""
	}
	if _, ok := aw.PromptAny().(ApprovalPrompt); ok {
		return "approval"
	}
	return "question"
}

// endTurn closes the turn's event channel and removes it from the live
// registry — subsequent Events / Cancel / Resume return ErrTurnNotFound.
func (s *inMemory) endTurn(st *turnState) {
	close(st.events)
	s.mu.Lock()
	delete(s.turns, st.handle.TurnID)
	s.mu.Unlock()
}

// finishTurn emits the terminal [TurnEnd] (stamping the elapsed duration)
// and tears the turn down. It serves the emergency-teardown paths —
// [Service.Cancel] and a failed [Service.Resume] — where no drive
// goroutine will run [emitTurnEnd]. The clean path goes through
// emitTurnEnd (which carries usage) followed by endTurn in [drive].
func (s *inMemory) finishTurn(st *turnState, reason TurnEndReason) {
	s.emit(st, TurnEnd{Reason: reason, Duration: time.Since(st.startedAt)})
	s.endTurn(st)
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
	out, _ := proc.Output()
	plan := terminalPlan(terminal, out, runErr, ctxErr, proc.Status())

	if plan.errMsg != "" {
		s.emit(st, ErrorEvent{Message: plan.errMsg, Code: plan.errCode})
	}
	end := TurnEnd{Reason: plan.reason, Duration: duration}
	if plan.withUsage {
		end.TokenUsage = out.Usage
		end.UsageByModel = out.UsageByModel
		end.CostUSD = out.CostUSD
	}
	s.emit(st, end)
}

// turnEndPlan is the decision emitTurnEnd derives before emitting: the
// TurnEnd reason, whether the turn's usage should ride along (only clean
// / budget-stopped completions carry usage; cancellations and errors
// don't), and an optional ErrorEvent to emit first.
type turnEndPlan struct {
	reason    TurnEndReason
	withUsage bool
	errMsg    string // non-empty → emit an ErrorEvent before TurnEnd
	errCode   string
}

// terminalPlan maps the captured agent-runtime terminal event onto a
// turnEndPlan. The lifecycle listener fires terminal events
// authoritatively (ProcessCompleted / Killed / Failed / Stuck /
// Terminated), so those drive the decision; the default case is the
// fallback for stub tests where no listener fired and the race where
// Done() returned before the platform multicast delivered the event.
func terminalPlan(terminal event.Event, out engine.ChatOutput, runErr, ctxErr error, status core.AgentProcessStatus) turnEndPlan {
	switch t := terminal.(type) {
	case event.ProcessCompleted:
		return completedPlan(out)
	case event.ProcessKilled, event.ProcessTerminated:
		return turnEndPlan{reason: TurnEndCancelled}
	case event.ProcessFailed:
		msg := "engine error"
		if t.Err != nil {
			msg = t.Err.Error()
		}
		return turnEndPlan{reason: TurnEndErrored, errMsg: msg, errCode: "ENGINE_ERROR"}
	case event.ProcessStuck:
		return turnEndPlan{reason: TurnEndErrored, errMsg: "agent stuck — planner produced no plan", errCode: "AGENT_STUCK"}
	default:
		return fallbackPlan(out, runErr, ctxErr, status)
	}
}

// completedPlan maps a cleanly-completed turn's output to its reason: a
// rejected plan is a (usage-free) cancellation, a budget stop is its own
// reason, otherwise a plain completion. Shared by the ProcessCompleted
// case and the fallback so the mapping lives in one place.
func completedPlan(out engine.ChatOutput) turnEndPlan {
	switch {
	case out.PlanRejected:
		return turnEndPlan{reason: TurnEndCancelled}
	case out.StoppedOnBudget:
		return turnEndPlan{reason: TurnEndBudgetExceeded, withUsage: true}
	default:
		return turnEndPlan{reason: TurnEndCompleted, withUsage: true}
	}
}

// fallbackPlan derives the plan from the run-loop signals when no
// terminal event was captured: a run error is a cancellation (ctx
// canceled / killed) or an engine error; no error falls back to the
// same completion mapping the happy path uses.
func fallbackPlan(out engine.ChatOutput, runErr, ctxErr error, status core.AgentProcessStatus) turnEndPlan {
	if runErr != nil {
		if status == core.StatusKilled || errors.Is(ctxErr, context.Canceled) {
			return turnEndPlan{reason: TurnEndCancelled}
		}
		return turnEndPlan{reason: TurnEndErrored, errMsg: runErr.Error(), errCode: "ENGINE_ERROR"}
	}
	return completedPlan(out)
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
