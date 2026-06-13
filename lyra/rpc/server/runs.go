package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine/chat"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/domain/session"
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
func (s *Server) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	sessionID, err := s.resolveSession(ctx, in.SessionID)
	if err != nil {
		return nil, nil, err
	}

	// The turn's filesystem + bash tools run in the session's project cwd
	// (API.md §0.2). Resolve it here so the engine anchors them per session
	// rather than at the single serve-time workdir.
	sess, err := s.rt.Session().Get(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}

	userMsg := joinUserText(in.Input)
	if userMsg == "" {
		return nil, nil, errors.New("runs.start: input must contain a user text block")
	}

	// providerId + model are paired: both to pick a model, neither for the
	// default. One without the other is a self-contradictory request — the
	// provider is explicit, never inferred (API.md §7.3).
	if (in.Model == "") != (in.Provider == "") {
		return nil, nil, protocol.ErrInvalidParams
	}

	// Mode (agent|chat|plan, API.md §7.1): agent is the default tool loop;
	// plan threads to the chat service's plan-preview flow; chat runs
	// tool-less (a plain single-round exchange). Unknown values are
	// invalid_params, never silently dropped.
	planMode, chatMode := false, false
	switch in.Mode {
	case "", protocol.RunModeAgent:
	case protocol.RunModePlan:
		planMode = true
	case protocol.RunModeChat:
		chatMode = true
	default:
		return nil, nil, fmt.Errorf("%w: unknown mode %q", protocol.ErrInvalidParams, in.Mode)
	}

	handle, err := s.rt.Chat().StartTurn(ctx, chat.StartTurnRequest{
		SessionID:  sessionID,
		Message:    userMsg,
		Cwd:        sess.Cwd,
		Provider:   in.Provider,
		Model:      in.Model,
		PlanMode:   planMode,
		ChatMode:   chatMode,
		MaxCostUSD: in.MaxBudgetUSD,
	})
	if err != nil {
		return nil, nil, err
	}

	// Record the model the run explicitly selected so sessions.list / get
	// surface the session's current model (Session.model). An unset model
	// runs the default — sessionToWire fills that from the runtime default.
	if in.Model != "" {
		_ = s.rt.Session().SetModel(ctx, sessionID, in.Model)
	}

	// runId on the wire == the turn id for the root run. The user's input
	// rides the stream as the run's opening userMessage Item (translator
	// emits it after run.started) — streamed live and persisted through the
	// same path, so the wire id and the items.list id are one and the same.
	runID := handle.TurnID
	out, events, err := s.openSegment(ctx, runID, "", handle, sessionID, in.Input, nil)
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
func (s *Server) ResumeRun(ctx context.Context, in protocol.ResumeRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	pending, ok, err := s.rt.Interrupts().Get(ctx, in.ParentRunID)
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
	if err = s.rt.Chat().Resume(ctx, handle, resolution); err != nil {
		if !errors.Is(err, chat.ErrTurnNotFound) {
			return nil, nil, err
		}
		// The live turn is gone (the backend restarted). Rebuild the parked
		// process from its persisted snapshot and resume the continuation on
		// a fresh turn. Needs a recorded ProcessID + a configured durable
		// ProcessStore; if either is missing the interrupt is genuinely
		// unresumable, so drop it (API.md §6.2 anti-dangling).
		rebuilt, rerr := s.rehydrate(ctx, pending, resolution.Approved)
		if rerr != nil {
			// Best-effort: rehydrate failed; drop the unresumable
			// interrupt record so it won't show up in list queries.
			_ = s.rt.Interrupts().Delete(ctx, in.ParentRunID)
			return nil, nil, protocol.ErrRunNotFound
		}
		handle = rebuilt
	}
	// Best-effort: the interrupt is now answered — drop the
	// open-interrupt record. If this fails, a dangling record
	// surfaces on the next list and can be re-resumed safely.
	_ = s.rt.Interrupts().Delete(ctx, in.ParentRunID)

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
	out, events, err := s.openSegment(ctx, contRunID, in.ParentRunID, handle, pending.SessionID, nil, resumeBindingFrom(pending))
	if err != nil {
		return nil, nil, err
	}
	return out, events, nil
}

// rehydrate rebuilds a parked turn whose live state was lost on restart,
// from its persisted process snapshot, and resumes it with the decision.
// Returns the fresh turn handle the continuation streams on, or an error
// when the interrupt can't be rebuilt (no recorded ProcessID, no
// ProcessStore, or a missing / non-deployable snapshot).
func (s *Server) rehydrate(ctx context.Context, pending interrupts.Pending, approved bool) (chat.TurnHandle, error) {
	if pending.ProcessID == "" {
		return chat.TurnHandle{}, errors.New("server: interrupt has no recorded process id")
	}
	return s.rt.Chat().Rehydrate(ctx, chat.RehydrateRequest{
		SessionID: pending.SessionID,
		ProcessID: pending.ProcessID,
		Approved:  approved,
	})
}

// CancelRun hard-stops a running run (outcome:canceled, API.md §7.3).
// A parked run is also abandoned — its live parked turn is torn down
// and its open interrupt dropped so it stops surfacing as resumable.
func (s *Server) CancelRun(ctx context.Context, in protocol.CancelRunRequest) error {
	s.runMu.Lock()
	e, ok := s.runs[in.RunID]
	if ok {
		e.cancelReason = in.Reason // surfaced on the synthesized canceled outcome (S6)
	}
	s.runMu.Unlock()

	if !ok {
		// Not actively pumping — a parked run whose pump already returned.
		// The open-interrupt record maps the run back to its live parked
		// turn: cancel that turn first (tears down the parked process and
		// turn state), THEN drop the record. Resolving before deleting
		// keeps the operation atomic from the client's view — a failed
		// lookup leaves the run resumable instead of half-abandoned.
		pending, found, err := s.rt.Interrupts().Get(ctx, in.RunID)
		if err != nil || !found {
			return protocol.ErrRunNotFound
		}
		_ = s.rt.Chat().Cancel(ctx, chat.TurnHandle{SessionID: pending.SessionID, TurnID: pending.TurnID})
		_ = s.rt.Interrupts().Delete(ctx, in.RunID)
		return nil
	}

	// Actively pumping: drop any open interrupt record (no-op for an
	// un-parked run), cancel the run ctx, and stop the underlying turn.
	_ = s.rt.Interrupts().Delete(ctx, in.RunID)
	e.cancel()
	_ = s.rt.Chat().Cancel(ctx, chat.TurnHandle{SessionID: e.sessionID, TurnID: e.turnID})
	return nil
}

// ListRuns returns the currently running runs as a Page (API.md §7.3).
// The set is in-process and bounded, so the page carries no cursor.
func (s *Server) ListRuns(_ context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	out := make([]protocol.RunRef, 0, len(s.runs))
	for _, e := range s.runs {
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
	return protocol.NewPage(out), nil
}

// ListOpenInterrupts returns durable resumable interrupts as a Page
// (API.md §6.2), read from the pluggable interrupt store.
func (s *Server) ListOpenInterrupts(ctx context.Context, in protocol.ListOpenInterruptsRequest) (*protocol.Page[protocol.OpenInterrupt], error) {
	pending, err := s.rt.Interrupts().List(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.OpenInterrupt, 0, len(pending))
	for _, p := range pending {
		var ints []protocol.Interrupt
		if err := json.Unmarshal(p.Interrupts, &ints); err != nil {
			// Corrupted interrupt record — skip it rather than
			// surfacing a bogus entry with zero interrupts.
			continue
		}
		out = append(out, protocol.OpenInterrupt{
			ParentRunID: p.ParentRunID,
			SessionID:   p.SessionID,
			Interrupts:  ints,
			CreatedAt:   p.CreatedAt,
		})
	}
	return protocol.NewPage(out), nil
}

// SubscribeRun opens a fresh event stream onto an actively-streaming root
// run (reconnect / crash recovery; subscribes the whole run tree, API.md
// §5.4 / §7.3). It attaches a new subscriber to the run's hub, replaying
// the durable backlog after the caller's Last-Event-Id (carried out-of-band
// via ctx, TRANSPORT §9.2) then tailing live. A run that isn't actively
// streaming (finished / parked / unknown) returns run_not_found — its tail
// is recovered through items.list, not here.
func (s *Server) SubscribeRun(ctx context.Context, runID string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	if runID == "" {
		return nil, nil, protocol.ErrRunNotFound
	}
	s.runMu.Lock()
	e, live := s.runs[runID]
	s.runMu.Unlock()
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
// [interrupts.Resolution] the chat service's Resume expects. The agent
// runtime parks one awaitable at a time, so a single response drives the
// continuation. approval → approve/deny; answer → the answers map (and
// approve unless the plan-review label is reject); toolResult / empty →
// continue.
func resolveResolution(responses []protocol.InterruptResponse) (interrupts.Resolution, error) {
	for _, r := range responses {
		switch r.Response.Type {
		case "approval":
			// remember{scope:session} keeps the decision for the session; any
			// other scope isn't persisted yet, so we honor it as one-shot
			// rather than promise a memory we can't keep (AUX_API §6).
			res := interrupts.Resolution{
				Remember: r.Response.Remember != nil && r.Response.Remember.Scope == "session",
			}
			switch r.Response.Decision {
			case "approve":
				res.Approved = true
				// editedArgs overrides the model-regenerated tool args for this
				// one call (the gate's verdict.Arguments path). One-shot: never
				// folded into a remembered decision.
				if len(r.Response.EditedArgs) > 0 {
					b, err := json.Marshal(r.Response.EditedArgs)
					if err != nil {
						return interrupts.Resolution{}, fmt.Errorf("runs.resume: editedArgs: %w", err)
					}
					res.Arguments = string(b)
				}
			case "deny":
				res.Approved = false
			default:
				return interrupts.Resolution{}, errors.New(`runs.resume: approval decision must be "approve" | "deny"`)
			}
			return res, nil
		case "answer":
			// Plan-review question (see translator.questionInterrupt): the
			// decision field carries the chosen label (a single-element array,
			// S8). Anything other than an explicit reject proceeds. ask_user
			// answers ride the same map.
			approved := true
			if v := r.Response.Answers[planDecisionField]; len(v) > 0 && v[0] == planDecisionReject {
				approved = false
			}
			return interrupts.Resolution{Approved: approved, Answer: r.Response.Answers}, nil
		case "toolResult":
			return interrupts.Resolution{Approved: true}, nil
		}
	}
	// No actionable response → treat as continue.
	return interrupts.Resolution{Approved: true}, nil
}

// resolveSession verifies sessionID exists, or creates a fresh session
// when empty (zero-friction cold start for in-process callers).
func (s *Server) resolveSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		sess, err := s.rt.Session().Create(ctx, "", s.serverInfo.Cwd)
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := s.rt.Session().Get(ctx, sessionID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", protocol.ErrSessionNotFound
		}
		return "", err
	}
	return sessionID, nil
}

// joinUserText joins ALL of a run's input text blocks (newline-
// separated) into the single user message the in-process
// chat.StartTurn API expects today.
func joinUserText(blocks []protocol.ContentBlock) string {
	var b []string
	for _, blk := range blocks {
		if blk.Type == "text" && blk.Text != "" {
			b = append(b, blk.Text)
		}
	}
	return strings.Join(b, "\n")
}
