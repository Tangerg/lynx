package server

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// StartRun translates runs.start into the in-process chat.StartTurn
// path (API.md §7.3). It returns the runId synchronously; events flow
// out via the returned channel as RunEvents (wrapped by the transport
// into notifications.run.event). The terminal run.finished rides this
// channel — including outcome:interrupt when the run parks for HITL
// approval, after which the run suspends and the client answers via
// runs.resume.
func (i *Server) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	sessionID, err := i.resolveSession(ctx, in.SessionID)
	if err != nil {
		return nil, nil, err
	}

	// The turn's filesystem + bash tools run in the session's project cwd
	// (API.md §0.2). Resolve it here so the engine anchors them per session
	// rather than at the single serve-time workdir.
	sess, err := i.rt.Session().Get(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}

	userMsg := lastUserText(in.Input)
	if userMsg == "" {
		return nil, nil, errors.New("runs.start: input must contain a user text block")
	}

	// providerId + model are paired: both to pick a model, neither for the
	// default. One without the other is a self-contradictory request — the
	// provider is explicit, never inferred (API.md §7.3).
	if (in.Model == "") != (in.Provider == "") {
		return nil, nil, protocol.ErrInvalidParams
	}

	handle, err := i.rt.Chat().StartTurn(ctx, chat.StartTurnRequest{
		SessionID:  sessionID,
		Message:    userMsg,
		Cwd:        sess.Cwd,
		Provider:   in.Provider,
		Model:      in.Model,
		MaxCostUSD: in.MaxBudgetUSD,
	})
	if err != nil {
		return nil, nil, err
	}

	// Record the model the run explicitly selected so sessions.list / get
	// surface the session's current model (Session.model). An unset model
	// runs the default — sessionToWire fills that from the runtime default.
	if in.Model != "" {
		_ = i.rt.Session().SetModel(ctx, sessionID, in.Model)
	}

	// runId on the wire == the turn id for the root run. The user's input
	// rides the stream as the run's opening userMessage Item (translator
	// emits it after run.started) — streamed live and persisted through the
	// same path, so the wire id and the items.list id are one and the same.
	runID := handle.TurnID
	out, events, err := i.openSegment(ctx, runID, "", handle, sessionID, in.Input, nil)
	if err != nil {
		return nil, nil, err
	}
	// Return the opening userMessage Item id so the client reconciles its
	// optimistic bubble by exact id (same id the stream + items.list carry).
	out.UserItemID = userMessageItemID(runID)
	return out, events, nil
}

// ResumeRun answers an open interrupt by continuing the parked run as a
// fresh continuation run (R model, API.md §6). parentRunId identifies
// the interrupted run; the response decision is delivered to the live
// agent process, and the continuation streams under a new runId linked
// back via RunRef.parentRunId.
func (i *Server) ResumeRun(ctx context.Context, in protocol.ResumeRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	pending, ok, err := i.rt.Interrupts().Get(ctx, in.ParentRunID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, protocol.ErrInterruptNotOpen
	}
	resolution, err := resolveResolution(in.Responses)
	if err != nil {
		return nil, nil, err
	}

	handle := chat.TurnHandle{SessionID: pending.SessionID, TurnID: pending.TurnID}
	if err := i.rt.Chat().Resume(ctx, handle, resolution); err != nil {
		if !errors.Is(err, chat.ErrTurnNotFound) {
			return nil, nil, err
		}
		// The live turn is gone (the backend restarted). Rebuild the parked
		// process from its persisted snapshot and resume the continuation on
		// a fresh turn. Needs a recorded ProcessID + a configured durable
		// ProcessStore; if either is missing the interrupt is genuinely
		// unresumable, so drop it (API.md §6.2 anti-dangling).
		rebuilt, rerr := i.rehydrate(ctx, pending, resolution.Approved)
		if rerr != nil {
			_ = i.rt.Interrupts().Delete(ctx, in.ParentRunID)
			return nil, nil, protocol.ErrRunNotFound
		}
		handle = rebuilt
	}
	// The interrupt is now answered — drop the open-interrupt record
	// before streaming the continuation.
	_ = i.rt.Interrupts().Delete(ctx, in.ParentRunID)

	// Continuation gets a fresh wire runId linked to the parent. handle.TurnID
	// is the original turn for a same-process resume, or the freshly rebuilt
	// turn for a cross-restart one — and already carries the run_ prefix, so
	// we suffix it (not re-prefix) to derive a distinct continuation id.
	contRunID := handle.TurnID + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	// A continuation carries no new user turn — the decision is delivered
	// out-of-band via runs.resume, so no opening userMessage Item. It DOES
	// carry the resume binding: an approved tool re-fires in this run and
	// must complete its ORIGINAL proposal item (API.md §5.2 / §6), not a
	// fresh one.
	out, events, err := i.openSegment(ctx, contRunID, in.ParentRunID, handle, pending.SessionID, nil, resumeBindingFrom(pending))
	if err != nil {
		return nil, nil, err
	}
	return out, events, nil
}

// resumeBindingFrom extracts the pending approval items' ids (keyed by tool
// name + arguments) from a parked run so the continuation translator can
// reuse them when the approved tools re-fire. Returns nil when there are no
// approval interrupts (e.g. a plan-review question, which resolves without a
// re-fired tool). originRunID is the interrupted run the items were created
// in — the continuation re-emits them under that run so the item's id + runId
// stay stable across the boundary.
func resumeBindingFrom(pending interrupts.Pending) *resumeBinding {
	var ints []protocol.Interrupt
	if err := json.Unmarshal(pending.Interrupts, &ints); err != nil || len(ints) == 0 {
		return nil
	}
	items := map[string]string{}
	var questions []resumedQuestion
	for _, in := range ints {
		if in.ItemID == "" {
			continue
		}
		switch in.Kind {
		case "approval":
			// Re-bind via the backend-internal _resume tuple (raw name +
			// arguments) the translator stashed — payload.tool is the display
			// ToolInvocation and drops the name on strongly-typed variants.
			rm, _ := in.Payload["_resume"].(map[string]any)
			tool, _ := rm["name"].(string)
			args, _ := rm["args"].(string)
			if tool != "" {
				items[resumeKey(tool, args)] = in.ItemID
			}
		case "question":
			// A plan-review question is resolved by the resume answer (no
			// re-fired event), so the continuation must complete its item.
			questions = append(questions, resumedQuestion{itemID: in.ItemID, question: questionFromPayload(in.Payload)})
		}
	}
	if len(items) == 0 && len(questions) == 0 {
		return nil
	}
	return &resumeBinding{originRunID: pending.ParentRunID, toolItems: items, questions: questions}
}

// questionFromPayload reconstructs the wire Question from an interrupt's
// payload map (round-tripped through JSON in the interrupt store) so the
// continuation's terminal item.completed carries the same content the
// proposal did. Returns nil when absent / malformed (the item still
// completes — just without re-stated content; the client already has it).
func questionFromPayload(payload map[string]any) *protocol.Question {
	raw, ok := payload["question"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var q protocol.Question
	if err := json.Unmarshal(b, &q); err != nil {
		return nil
	}
	return &q
}

// rehydrate rebuilds a parked turn whose live state was lost on restart,
// from its persisted process snapshot, and resumes it with the decision.
// Returns the fresh turn handle the continuation streams on, or an error
// when the interrupt can't be rebuilt (no recorded ProcessID, no
// ProcessStore, or a missing / non-deployable snapshot).
func (i *Server) rehydrate(ctx context.Context, pending interrupts.Pending, approved bool) (chat.TurnHandle, error) {
	if pending.ProcessID == "" {
		return chat.TurnHandle{}, errors.New("server: interrupt has no recorded process id")
	}
	return i.rt.Chat().Rehydrate(ctx, chat.RehydrateRequest{
		SessionID: pending.SessionID,
		ProcessID: pending.ProcessID,
		Approved:  approved,
	})
}

// openSegment subscribes to the turn's event stream and starts the wire
// pump for one run segment. parentRunID is empty for a root run
// (runs.start) and set for a continuation (runs.resume) — it rides onto
// the RunRef and the runEntry so the continuation links back to its parent.
func (i *Server) openSegment(reqCtx context.Context, runID, parentRunID string, handle chat.TurnHandle, sessionID string, userInput []protocol.ContentBlock, resume *resumeBinding) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	// Detach the run from the request's cancellation (it must outlive the
	// request) WITHOUT losing the request's trace context: WithoutCancel
	// keeps ctx values — including the entry span — so the run's spans are
	// children of the same trace (full-link), while our own cancel drives
	// CancelRun. Rooting on context.Background() here would sever the trace.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(reqCtx))
	inner, err := i.rt.Chat().Events(runCtx, handle)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	// The hub owns the run's event stream for its whole lifetime,
	// independent of any client connection (streamable HTTP, TRANSPORT
	// §6.4/§9.2). The pump appends to it; this caller streams from a
	// fresh subscription; a later runs.subscribe attaches another.
	hub := newRunHub()
	i.runMu.Lock()
	i.runs[runID] = &runEntry{runID: runID, sessionID: sessionID, turnID: handle.TurnID, parentRunID: parentRunID, cancel: cancel, hub: hub}
	i.runMu.Unlock()
	events, unsubscribe := hub.Subscribe("")
	// Drop this caller's subscription when its request ends (client
	// disconnect or stream completion) — the run keeps running on runCtx
	// and stays resumable via runs.subscribe. runCtx is Background-rooted
	// on purpose: the run must outlive the request that started it.
	context.AfterFunc(reqCtx, unsubscribe)
	go i.pumpRun(runCtx, runID, parentRunID, handle, inner, hub, userInput, resume)
	return &protocol.StartRunResponse{RunID: runID}, events, nil
}

// pumpRun translates the turn's chat events to RunEvents under runID.
// A run segment ends one of two ways:
//
//   - interrupt: the turn parked for HITL approval. The pump records an
//     open interrupt (so runs.listOpenInterrupts finds it) and returns,
//     leaving the live turn parked for runs.resume.
//   - terminal: the turn finished. The pump tears the run down.
//
// Either way the wire run.finished event is the last thing on the
// channel before it closes.
func (i *Server) pumpRun(ctx context.Context, runID, parentRunID string, handle chat.TurnHandle, inner iter.Seq[chat.Event], hub *runHub, userInput []protocol.ContentBlock, resume *resumeBinding) {
	tr := newTranslator(handle.SessionID, runID, parentRunID, userInput, resume)
	finished := false
	parked := false

	// emit assigns each StreamEvent its eventId, persists the durable
	// side to history, and appends to the hub. hub.Append is non-blocking
	// (it drops on a slow subscriber, never stalls the run), so unlike the
	// old per-connection channel there is no client-disconnect backpressure
	// here — the run streams to the hub regardless of who is listening.
	emit := func(events []protocol.StreamEvent) {
		for _, se := range events {
			re := protocol.RunEvent{
				RunID:     runID,
				EventID:   i.nextEventID(),
				Timestamp: time.Now().UTC(),
				Durable:   durableFor(se.Type),
				Event:     se,
			}
			if se.Type == protocol.StreamRunFinished {
				finished = true
				if se.Outcome != nil && se.Outcome.Type == protocol.OutcomeInterrupt {
					parked = true
					i.recordInterrupt(ctx, runID, handle, se.Outcome.Interrupts)
				}
			}
			// Persist with a background ctx so the durable history (incl.
			// the terminal run.finished synthesized on a canceled run) lands
			// regardless of run-ctx cancellation.
			i.persistStreamEvent(context.Background(), runID, handle.SessionID, parentRunID, se)
			hub.Append(re)
		}
	}

	// run.started leads every segment (root + continuation), independent of
	// any chat-level TurnStart — continuation runs (runs.resume) carry none,
	// so emitting here is what gives them a run boundary + parentRunId, and
	// closes any plan-review question the parked run left open.
	emit(tr.open())

	defer func() {
		if !finished {
			// The turn ended without a run.finished (canceled mid-flight /
			// drained iterator) — synthesize the terminal so the stream ends
			// balanced.
			outcome := protocol.OutcomeCanceled
			if tr.errMsg != "" {
				outcome = protocol.OutcomeError
			}
			emit(tr.finish(outcome))
		}
		hub.Close()
		i.runMu.Lock()
		if e, ok := i.runs[runID]; ok {
			// A parked run keeps its live turn alive for resume — only
			// cancel + forget on a true terminal.
			if !parked {
				e.cancel()
			}
			delete(i.runs, runID)
		}
		i.runMu.Unlock()
	}()

	for ev := range inner {
		emit(tr.translate(ev))
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
	}
}

// recordInterrupt persists the open interrupt so runs.listOpenInterrupts
// can discover it and runs.resume can map it back to the live turn.
func (i *Server) recordInterrupt(ctx context.Context, runID string, handle chat.TurnHandle, ints []protocol.Interrupt) {
	raw, err := json.Marshal(ints)
	if err != nil {
		return
	}
	// Capture the parked process's snapshot key so a restart can rebuild
	// it (cross-restart resume). Best-effort: an empty id just means resume
	// stays same-process-only for this interrupt.
	processID, _ := i.rt.Chat().ProcessID(ctx, handle)
	_ = i.rt.Interrupts().Put(ctx, interrupts.Pending{
		ParentRunID: runID,
		SessionID:   handle.SessionID,
		TurnID:      handle.TurnID,
		ProcessID:   processID,
		Interrupts:  raw,
		CreatedAt:   time.Now().UTC(),
	})
}

// durableFor classifies a stream event's durability (API.md §5.3).
func durableFor(t protocol.StreamEventType) bool {
	switch t {
	case protocol.StreamItemDelta, protocol.StreamStateDelta:
		return false
	default:
		return true
	}
}

// CancelRun hard-stops a running run (outcome:canceled, API.md §7.3).
// A parked run is also abandoned — its open interrupt is dropped so it
// stops surfacing as resumable.
func (i *Server) CancelRun(ctx context.Context, in protocol.CancelRunRequest) error {
	i.runMu.Lock()
	e, ok := i.runs[in.RunID]
	i.runMu.Unlock()
	_ = i.rt.Interrupts().Delete(ctx, in.RunID)
	if !ok {
		// Not actively running — it may be a parked run whose pump already
		// returned. Cancel the underlying turn by id if we can find it via
		// the interrupt record (already deleted above); best-effort.
		return protocol.ErrRunNotFound
	}
	e.cancel()
	_ = i.rt.Chat().Cancel(ctx, chat.TurnHandle{SessionID: e.sessionID, TurnID: e.turnID})
	return nil
}

// ListRuns returns the currently running runs (API.md §7.3).
func (i *Server) ListRuns(_ context.Context, in protocol.ListRunsRequest) ([]protocol.RunRef, error) {
	i.runMu.Lock()
	defer i.runMu.Unlock()
	out := make([]protocol.RunRef, 0, len(i.runs))
	for _, e := range i.runs {
		if in.SessionID != "" && e.sessionID != in.SessionID {
			continue
		}
		out = append(out, protocol.RunRef{
			ID:          e.runID,
			SessionID:   e.sessionID,
			ParentRunID: e.parentRunID,
			Status:      protocol.RunStatusRunning,
		})
	}
	return out, nil
}

// ListOpenInterrupts returns durable resumable interrupts (API.md §6.2),
// read from the pluggable interrupt store.
func (i *Server) ListOpenInterrupts(ctx context.Context, in protocol.ListOpenInterruptsRequest) ([]protocol.OpenInterrupt, error) {
	pending, err := i.rt.Interrupts().List(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.OpenInterrupt, 0, len(pending))
	for _, p := range pending {
		var ints []protocol.Interrupt
		_ = json.Unmarshal(p.Interrupts, &ints)
		out = append(out, protocol.OpenInterrupt{
			ParentRunID: p.ParentRunID,
			SessionID:   p.SessionID,
			Interrupts:  ints,
			CreatedAt:   p.CreatedAt,
		})
	}
	return out, nil
}

// SubscribeRun opens a fresh event stream onto an actively-streaming root
// run (reconnect / crash recovery; subscribes the whole run tree, API.md
// §5.4 / §7.3). It attaches a new subscriber to the run's hub, replaying
// the durable backlog after the caller's Last-Event-Id (carried out-of-band
// via ctx, TRANSPORT §9.2) then tailing live. A run that isn't actively
// streaming (finished / parked / unknown) returns run_not_found — its tail
// is recovered through items.list, not here.
func (i *Server) SubscribeRun(ctx context.Context, runID string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	if runID == "" {
		return nil, nil, protocol.ErrRunNotFound
	}
	i.runMu.Lock()
	e, live := i.runs[runID]
	i.runMu.Unlock()
	if !live || e.hub == nil {
		return nil, nil, protocol.ErrRunNotFound
	}
	events, unsubscribe := e.hub.Subscribe(transport.LastEventIDFrom(ctx))
	// Drop the subscription when this request ends; the run continues.
	context.AfterFunc(ctx, unsubscribe)
	return &protocol.StartRunResponse{RunID: runID}, events, nil
}

// ─── helpers ────────────────────────────────────────────────────────

// resolveResolution maps the wire interrupt responses onto the structured
// [engine.InterruptResolution] the chat service's Resume expects. The agent
// runtime parks one awaitable at a time, so a single response drives the
// continuation. approval → approve/deny; answer → the answers map (and
// approve unless the plan-review label is reject); toolResult / empty →
// continue.
func resolveResolution(responses []protocol.InterruptResponse) (engine.InterruptResolution, error) {
	for _, r := range responses {
		switch r.Response.Kind {
		case "approval":
			switch r.Response.Decision {
			case "approve":
				return engine.InterruptResolution{Approved: true}, nil
			case "deny":
				return engine.InterruptResolution{Approved: false}, nil
			default:
				return engine.InterruptResolution{}, errors.New(`runs.resume: approval decision must be "approve" | "deny"`)
			}
		case "answer":
			// Plan-review question (see translator.questionInterrupt): the
			// decision field carries the chosen label. Anything other than an
			// explicit reject proceeds. ask_user answers ride the same map.
			approved := true
			if v, ok := r.Response.Answers[planDecisionField]; ok {
				if s, _ := v.(string); s == planDecisionReject {
					approved = false
				}
			}
			return engine.InterruptResolution{Approved: approved, Answer: r.Response.Answers}, nil
		case "toolResult":
			return engine.InterruptResolution{Approved: true}, nil
		}
	}
	// No actionable response → treat as continue.
	return engine.InterruptResolution{Approved: true}, nil
}

// resolveSession verifies sessionID exists, or creates a fresh session
// when empty (zero-friction cold start for in-process callers).
func (i *Server) resolveSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		sess, err := i.rt.Session().Create(ctx, "", i.serverInfo.Cwd)
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := i.rt.Session().Get(ctx, sessionID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", protocol.ErrSessionNotFound
		}
		return "", err
	}
	return sessionID, nil
}

// lastUserText joins the text blocks of a run's input into the single
// user message the in-process chat.StartTurn API expects today.
func lastUserText(blocks []protocol.ContentBlock) string {
	var b []string
	for _, blk := range blocks {
		if blk.Type == "text" && blk.Text != "" {
			b = append(b, blk.Text)
		}
	}
	return strings.Join(b, "\n")
}
