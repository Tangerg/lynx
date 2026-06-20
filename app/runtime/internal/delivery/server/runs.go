package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"
	"github.com/Tangerg/lynx/pkg/mime"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
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

	userMsg, userMedia, err := collectUserInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	if userMsg == "" && len(userMedia) == 0 {
		return nil, nil, fmt.Errorf("%w: input must contain a user text or image block", protocol.ErrInvalidParams)
	}

	// providerId + model are paired: both to pick a model, neither for the
	// default. One without the other is a self-contradictory request — the
	// provider is explicit, never inferred (API.md §7.3).
	if (in.Model == "") != (in.Provider == "") {
		return nil, nil, protocol.ErrInvalidParams
	}

	// Image capability gate: when the run names an explicit provider+model,
	// refuse image input the model can't accept (catalog modalities) so the
	// user gets a clear error up front instead of a provider 400 mid-stream.
	// The default model (provider+model empty) is operator-configured and
	// skipped here; the frontend also gates via the per-model multimodal
	// capability (models.list). A catalog miss means unknown — let the
	// provider decide rather than block.
	if len(userMedia) > 0 && in.Provider != "" && in.Model != "" {
		if info, ok := catalog.Lookup(in.Provider, in.Model); ok && !info.Modalities.AcceptsInput(chat.ModalityImage) {
			return nil, nil, fmt.Errorf("%w: model %q (provider %q) does not accept image input", protocol.ErrInvalidParams, in.Model, in.Provider)
		}
	}

	handle, err := s.rt.Chat().StartTurn(ctx, turn.StartTurnRequest{
		SessionID:  sessionID,
		Message:    userMsg,
		Media:      userMedia,
		Cwd:        sess.Cwd,
		Provider:   in.Provider,
		Model:      in.Model,
		MaxCostUSD: in.MaxBudgetUSD,
		MaxSteps:   in.MaxSteps,
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
	out, events, err := s.openSegment(ctx, runID, "", handle, sessionID, in.Input, nil, in.Provider, in.Model)
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
	// Validate the decision BEFORE claiming the interrupt — a malformed
	// response shouldn't consume the (still-resumable) record.
	resolution, err := resolveResolution(in.Responses)
	if err != nil {
		return nil, nil, err
	}
	// Atomically claim the interrupt (read-and-remove in one op). This makes
	// resume idempotent: a second, concurrent resume of the same parentRunId
	// finds nothing here and backs off, so the parked process is never
	// rebuilt twice and the approved (possibly non-idempotent) tool never
	// re-fires. The claim commits us to resolving it — there's no leftover
	// record to re-resume whether resolution succeeds or fails.
	pending, ok, err := s.rt.Interrupts().Consume(ctx, in.ParentRunID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, protocol.ErrInterruptNotOpen
	}

	handle := turn.TurnHandle{SessionID: pending.SessionID, TurnID: pending.TurnID}
	if err = s.rt.Chat().Resume(ctx, handle, resolution); err != nil {
		if errors.Is(err, turn.ErrParkClaimed) {
			// A concurrent runs.cancel claimed the parked turn and is driving it
			// to canceled — don't rehydrate (that would resurrect the turn and
			// fire its pending tool). The interrupt is already consumed above, so
			// report it as no-longer-open; the cancel wins the race.
			return nil, nil, protocol.ErrInterruptNotOpen
		}
		if !errors.Is(err, turn.ErrTurnNotFound) {
			return nil, nil, err
		}
		// The live turn is gone (the backend restarted). Rebuild the parked
		// process from its persisted snapshot and resume the continuation on
		// a fresh turn. Needs a recorded ProcessID + a configured durable
		// ProcessStore; if either is missing the interrupt is genuinely
		// unresumable (and already claimed above, so nothing to drop).
		rebuilt, rerr := s.rehydrate(ctx, pending, resolution.Approved)
		if rerr != nil {
			return nil, nil, protocol.ErrRunNotFound
		}
		handle = rebuilt
	}

	// Continuation gets a fresh wire runId linked to the parent. handle.TurnID
	// is the original turn for a same-process resume, or the freshly rebuilt
	// turn for a cross-restart one — and already carries the run_ prefix, so
	// suffix it (not re-prefix) to derive a distinct continuation id.
	contRunID := handle.TurnID + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	// A continuation carries no new user turn — the decision is delivered
	// out-of-band via runs.resume, so no opening userMessage Item. It DOES
	// carry the resume binding: an approved tool re-fires in this run and
	// must complete its ORIGINAL proposal item (API.md §5.2 / §6), not a
	// fresh one.
	// A continuation runs against the parked run's same provider+model — stamp
	// them so the continuation's RunRef (and its persisted usage) is
	// self-describing, rather than forcing consumers to chase parentRunId.
	out, events, err := s.openSegment(ctx, contRunID, in.ParentRunID, handle, pending.SessionID, nil, resumeBindingFrom(pending), pending.Provider, pending.Model)
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
func (s *Server) rehydrate(ctx context.Context, pending interrupts.Pending, approved bool) (turn.TurnHandle, error) {
	if pending.ProcessID == "" {
		return turn.TurnHandle{}, errors.New("server: interrupt has no recorded process id")
	}
	return s.rt.Chat().Rehydrate(ctx, turn.RehydrateRequest{
		SessionID: pending.SessionID,
		ProcessID: pending.ProcessID,
		Approved:  approved,
		Provider:  pending.Provider,
		Model:     pending.Model,
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
		_ = s.rt.Chat().Cancel(ctx, turn.TurnHandle{SessionID: pending.SessionID, TurnID: pending.TurnID})
		_ = s.rt.Interrupts().Delete(ctx, in.RunID)
		return nil
	}

	// Actively pumping: tear down the turn FIRST (cancel the run ctx + stop the
	// underlying turn), THEN drop any open interrupt record — the same
	// cancel-then-delete order as the parked branch above. The inverse (delete
	// first) briefly leaves the record gone while the turn is still being torn
	// down, so a teardown failure would orphan a still-live turn with no
	// resumable record. Delete is a no-op for an un-parked run.
	e.cancel()
	_ = s.rt.Chat().Cancel(ctx, turn.TurnHandle{SessionID: e.sessionID, TurnID: e.turnID})
	_ = s.rt.Interrupts().Delete(ctx, in.RunID)
	return nil
}

// SteerRun injects a user message into an actively-running run so the model
// reads it on its next tool round (runs.steer, API.md §6). Only an
// actively-pumping run is steerable — a parked run (waiting on an interrupt)
// is answered via runs.resume, and a finished one can't be steered — so a
// miss in the live run registry is run_not_found.
func (s *Server) SteerRun(ctx context.Context, in protocol.SteerRunRequest) error {
	s.runMu.Lock()
	e, ok := s.runs[in.RunID]
	s.runMu.Unlock()
	if !ok {
		return protocol.ErrRunNotFound
	}
	// The run can finish between the registry read and the inject — or its
	// steering queue can close as the turn terminates (the run is still in
	// s.runs while the pump drains). InjectSteering reports both as
	// ErrTurnNotFound; map it to the wire run_not_found symbol so the client
	// retries the message as a fresh send rather than seeing it silently dropped.
	if err := s.rt.Chat().InjectSteering(ctx, turn.TurnHandle{SessionID: e.sessionID, TurnID: e.turnID}, in.Message); err != nil {
		if errors.Is(err, turn.ErrTurnNotFound) {
			return protocol.ErrRunNotFound
		}
		return err
	}
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
			Provider:    e.provider,
			Model:       e.model,
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
// continuation. approval → approve/deny; answer → the answers map (the
// answering tool, e.g. ask_user / exit_plan_mode, interprets it); toolResult
// → continue; an empty responses list → continue. An unrecognized response
// type is invalid_params, never a silent approve.
func resolveResolution(responses []protocol.InterruptResponse) (interrupts.Resolution, error) {
	for _, r := range responses {
		switch r.Response.Type {
		case protocol.InterruptResponseApproval:
			// remember{scope} persists this decision as a rule at the chosen
			// scope (session / project / global); the chat gate maps the scope
			// across and keys the rule (AUX_API §6). Empty = don't remember.
			res := interrupts.Resolution{}
			if r.Response.Remember != nil {
				scope, ok := rememberScopeFromWire(r.Response.Remember.Scope)
				if !ok {
					return interrupts.Resolution{}, fmt.Errorf("%w: remember scope must be %q | %q | %q", protocol.ErrInvalidParams, protocol.RememberSession, protocol.RememberProject, protocol.RememberGlobal)
				}
				res.RememberScope = scope
			}
			switch r.Response.Decision {
			case protocol.ApprovalApprove:
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
			case protocol.ApprovalDeny:
				res.Approved = false
			default:
				return interrupts.Resolution{}, fmt.Errorf(`%w: approval decision must be "approve" | "deny"`, protocol.ErrInvalidParams)
			}
			return res, nil
		case protocol.InterruptResponseAnswer:
			// A question answer (ask_user / exit_plan_mode): the answers map IS
			// the resolution — the answering tool reads its chosen label / fields
			// back and decides what to do (e.g. exit_plan_mode treats a "Reject"
			// label as "stay in plan mode"). Always continues; the decision lives
			// in the answer content, not in Approved.
			return interrupts.Resolution{Approved: true, Answer: r.Response.Answers}, nil
		case protocol.InterruptResponseToolResult:
			return interrupts.Resolution{Approved: true}, nil
		default:
			return interrupts.Resolution{}, fmt.Errorf("%w: unknown interrupt response type %q", protocol.ErrInvalidParams, r.Response.Type)
		}
	}
	// No responses → treat as continue.
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

// collectUserInput splits a run's input blocks into the turn's user message:
// all text blocks joined newline-separated, and all image blocks turned into
// core media (Mime parsed to a MIME, Data taken as the base64 payload). An
// image block with a missing / non-image mime or empty data is rejected as
// invalid_params rather than silently dropped, so a malformed attachment
// surfaces to the user instead of vanishing. Unknown block types are ignored
// (forward-compatible). Media flows to the model as UserMessage.Media; the
// original blocks ride the opening userMessage Item verbatim for echo/replay.
func collectUserInput(blocks []protocol.ContentBlock) (string, []*media.Media, error) {
	var (
		texts  []string
		images []*media.Media
	)
	for _, blk := range blocks {
		switch blk.Type {
		case protocol.ContentBlockText:
			if blk.Text != "" {
				texts = append(texts, blk.Text)
			}
		case protocol.ContentBlockImage:
			mt, err := mime.Parse(blk.Mime)
			if err != nil {
				return "", nil, fmt.Errorf("%w: image block has invalid mime %q", protocol.ErrUnsupportedMime, blk.Mime)
			}
			if !mime.IsImage(mt) {
				return "", nil, fmt.Errorf("%w: block mime %q is not an image type", protocol.ErrUnsupportedMime, blk.Mime)
			}
			if blk.Data == "" {
				return "", nil, fmt.Errorf("%w: image block has empty data", protocol.ErrInvalidParams)
			}
			// Data is the base64 payload as a string — the form both the
			// anthropic (NewImageBlockBase64) and openai-compatible adapters
			// read back via Media.DataAsString.
			m, err := media.NewMedia(mt, blk.Data)
			if err != nil {
				return "", nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
			}
			images = append(images, m)
		}
	}
	return strings.Join(texts, "\n"), images, nil
}
