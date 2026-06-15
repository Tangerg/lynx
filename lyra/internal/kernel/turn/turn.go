package turn

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	corechat "github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/kernel"
)

// turnState holds the per-turn bookkeeping the implementation needs:
// the event channel subscribers read from, the cancel func that fires
// when [Service.Cancel] is called, and a monotone sequence number stamped
// onto every emitted event.
//
// The turn owns its own synchronization: mu guards the cross-goroutine
// mutable state (the backing process, the parked flag, the steering queue),
// reached only through the methods below, so the service mutex is left to
// guard just the live-turn registry. The remaining fields are set once at
// the entry point and read without locking thereafter.
type turnState struct {
	handle TurnHandle
	events chan Event
	cancel context.CancelFunc
	seq    atomic.Uint64

	// cwd is the session working directory the turn ran in — threaded
	// to post-turn maintenance so extracted facts land in THAT
	// project's LYRA.md. Empty for turns without a session cwd (the
	// memory service then falls back to its default dir) and for
	// rehydrated turns (the snapshot predates the live request).
	cwd string

	// ctx is the turn's own lifetime context — derived via
	// context.WithoutCancel from the entry ctx so it outlives the
	// StartTurn caller's cancellation yet KEEPS the entry trace span, then
	// wrapped with the turn span so the engine's LLM / tool / agent spans
	// nest under one trace (full-link). It bounds the run, the resume
	// continuation, and post-turn maintenance; canceled by [Service.Cancel].
	// Set once at the entry point (StartTurn / Rehydrate).
	ctx context.Context

	// span is the business-level turn span (started at the entry point,
	// ended once in endTurn). Carried on ctx so child spans attach to it.
	span trace.Span

	// model is the resolved model name this turn runs against — stamped on
	// the span + metrics + logs. "default" when the turn didn't pick one.
	model string

	// startedAt stamps the turn's wall-clock start so TurnEnd carries a
	// duration that spans any interrupt/resume cycles.
	startedAt time.Time

	// lifecycle captures the process's authoritative terminal event;
	// retained across interrupt→resume so the eventual TurnEnd reads it.
	// Written once before the turn goroutine reads it; not mu-guarded.
	lifecycle *turnLifecycle

	// --- mu-guarded: mutated/read across the turn + caller goroutines ---
	mu sync.Mutex

	// proc is the agent process backing this turn, set once setProc dispatches
	// it. [Service.Cancel] / [Service.Resume] / [Service.ProcessID] read it
	// via process() from other goroutines.
	proc kernel.ChatProcess

	// parked is true while the turn is suspended on a HITL interrupt
	// (StatusWaiting) awaiting [Service.Resume]. A parked turn stays
	// registered (events channel open) until claimPark drives it to a
	// terminal state.
	parked bool

	// steering is the queue of mid-turn user messages injected via
	// [Service.InjectSteering]. The runtime flushes it to the chat-memory
	// store after the turn ends so the messages land in conversation
	// history for the next turn.
	steering []string
}

// setProc records the agent process backing this turn. runTurn / Rehydrate
// write it once they have dispatched one; process() then hands it to the
// caller goroutines.
func (st *turnState) setProc(p kernel.ChatProcess) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.proc = p
}

// process returns the backing agent process, or nil before the turn has
// dispatched one. The value is stable after the single setProc, so callers
// may invoke its methods after process() returns.
func (st *turnState) process() kernel.ChatProcess {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.proc
}

// park marks the turn suspended on a HITL interrupt awaiting Resume.
func (st *turnState) park() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.parked = true
}

// claimPark atomically tests-and-clears the parked flag, reporting whether
// THIS caller claimed the suspended turn. [Service.Resume] and [Service.Cancel]
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

// appendSteering pushes one user message onto the pending-steering queue.
func (st *turnState) appendSteering(message string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.steering = append(st.steering, message)
}

// drainSteering returns the queued steering messages and clears the queue,
// or nil when none is pending.
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

// runTurn starts the turn's agent process and drives its first run
// segment to a suspension point — a HITL interrupt (park) or a terminal
// state. Later segments are driven by [inMemory.Resume] through the
// shared [drive] loop. st.ctx (the turn's own lifetime) bounds the run.
func (s *inMemory) runTurn(req StartTurnRequest, st *turnState) {
	s.emit(st, TurnStart{Model: st.model})

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
	proc := s.engine.StartChat(st.ctx, kernel.RunChatRequest{
		SessionID:     req.SessionID,
		Message:       req.Message,
		Media:         req.Media,
		Cwd:           req.Cwd,
		MaxBudget:     req.MaxBudget,
		MaxCostUSD:    req.MaxCostUSD,
		ChatClient:    client,
		Observer:      observer,
		EventListener: st.lifecycle.listener(st.handle.TurnID),
	})
	// Record the root process id so the lifecycle gate keeps subtask
	// terminals (which fire first) from being mistaken for the turn's end.
	st.lifecycle.setRoot(proc.ID())
	st.setProc(proc)

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
	proc := st.process()

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
func (s *inMemory) handleWaiting(st *turnState, proc kernel.ChatProcess) {
	aw := proc.PendingAwaitable()
	if aw == nil || s.canSurface(interruptKind(aw)) {
		s.emitInterrupt(st, proc)
		return
	}
	// Client can't answer this kind — deliver a deny and drive the
	// continuation (resumeAndDrive streams the terminal on a resume error
	// and launches drive otherwise; the returned error is already surfaced
	// on the channel, so it's safe to drop here).
	_ = s.resumeAndDrive(st, interrupts.Resolution{Approved: false})
}

// emitInterrupt marks the turn parked and surfaces the pending HITL
// request as a [TurnInterrupted] event. The turn stays registered with
// its events channel open; [inMemory.Resume] drives the next segment.
func (s *inMemory) emitInterrupt(st *turnState, proc kernel.ChatProcess) {
	aw := proc.PendingAwaitable()
	st.park()
	if aw == nil {
		// Defensive: Waiting without a parked awaitable shouldn't happen;
		// surface an empty interrupt rather than silently dropping it.
		s.emit(st, TurnInterrupted{})
		return
	}
	kind := interruptKind(aw)
	recordInterruptMetric(st.ctx, kind)
	s.emit(st, TurnInterrupted{Interrupts: []Interrupt{{Kind: kind, Payload: aw.PromptAny()}}})
}

// interruptKind classifies the pending awaitable into the wire interrupt
// kind (API.md §6: "approval" | "question" | "toolResult"). An
// [ApprovalPrompt] payload is a gated tool call ("approval"); anything
// else is a structured question (ask_user / exit_plan_mode), which surfaces
// as a "question". Returns "" for a nil awaitable (treated as surfaceable so
// the defensive empty-interrupt path in emitInterrupt still fires).
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
// It also ends the turn span: the single teardown point, so the span
// closes exactly once no matter which terminal path (drive / finishTurn)
// reached it. finishTurnSpan has already stamped the outcome.
func (s *inMemory) endTurn(st *turnState) {
	if st.span != nil {
		st.span.End()
	}
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
	dur := time.Since(st.startedAt)
	finishTurnSpan(st.span, reason, TokenUsage{}, false, "")
	recordTurnDuration(st.ctx, reason, st.model, dur)
	s.emit(st, TurnEnd{Reason: reason, Duration: dur})
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
func (s *inMemory) emitTurnEnd(st *turnState, proc kernel.ChatProcess, terminal event.Event, runErr error, duration time.Duration, ctxErr error) {
	out, _ := proc.Output()
	plan := planTurnEnd(terminal, out, runErr, ctxErr, proc.Status())

	finishTurnSpan(st.span, plan.reason, out.Usage, plan.withUsage, plan.errMsg)
	recordTurnDuration(st.ctx, plan.reason, st.model, duration)
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

// planTurnEnd is the turnEndPlan constructor: it maps the captured
// agent-runtime terminal event (plus the engine output and the run-loop's
// error signals) onto the plan emitTurnEnd executes. The lifecycle listener
// fires terminal events authoritatively (ProcessCompleted / Killed / Failed /
// Stuck / Terminated), so those drive the decision; the default case is the
// fallback for stub tests where no listener fired and the race where Done()
// returned before the platform multicast delivered the event. completedPlan /
// fallbackPlan are the per-branch builders it delegates to.
func planTurnEnd(terminal event.Event, out kernel.ChatOutput, runErr, ctxErr error, status core.AgentProcessStatus) turnEndPlan {
	switch t := terminal.(type) {
	case event.ProcessCompleted:
		return completedPlan(out)
	case event.ProcessKilled, event.ProcessTerminated:
		return turnEndPlan{reason: TurnEndCanceled}
	case event.ProcessFailed:
		msg := "engine error"
		if t.Err != nil {
			msg = t.Err.Error()
		}
		return turnEndPlan{reason: TurnEndErrored, errMsg: msg, errCode: "ENGINE_ERROR"}
	case event.ProcessStuck:
		return turnEndPlan{reason: TurnEndErrored, errMsg: "agent stuck — no forward progress", errCode: "AGENT_STUCK"}
	default:
		return fallbackPlan(out, runErr, ctxErr, status)
	}
}

// completedPlan maps a cleanly-completed turn's output to its reason: a
// budget stop is its own reason, otherwise a plain completion. Shared by
// the ProcessCompleted case and the fallback so the mapping lives in one
// place.
func completedPlan(out kernel.ChatOutput) turnEndPlan {
	switch {
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
func fallbackPlan(out kernel.ChatOutput, runErr, ctxErr error, status core.AgentProcessStatus) turnEndPlan {
	if runErr != nil {
		if status == core.StatusKilled || errors.Is(ctxErr, context.Canceled) {
			return turnEndPlan{reason: TurnEndCanceled}
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

	extraction, err := s.engine.MaybeExtract(ctx, sessionID, st.cwd)
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
