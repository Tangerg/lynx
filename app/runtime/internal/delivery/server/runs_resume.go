package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// ResumeRun answers an open interrupt by continuing the parked run as a
// fresh continuation run (R model, API.md §6). parentRunId identifies
// the interrupted run; the response decision is delivered to the live
// agent process, and the continuation streams under a new runId linked
// back via RunRef.parentRunId.
func (s *Server) ResumeRun(ctx context.Context, in protocol.ResumeRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	// Validate the decision BEFORE touching the interrupt — a malformed
	// response shouldn't consume the (still-resumable) record.
	resolution, err := resolveResolution(in.Responses)
	if err != nil {
		return nil, nil, err
	}
	// Peek the interrupt to learn its session, then claim the session's
	// single-writer slot BEFORE consuming the record. Consume removes the only
	// busy-marker a parked run has (it already left s.runs), so without the claim
	// a concurrent runs.start could slip into the window between Consume and the
	// continuation's openSegment. The claim is held through openSegment (released
	// on return). A concurrent resume of the same interrupt loses the claim and
	// is reported busy; Consume's atomic read-and-remove is the backstop so the
	// parked process is never rebuilt twice (the approved, possibly
	// non-idempotent, tool never re-fires).
	peek, found, err := s.rt.Interrupts().Get(ctx, in.ParentRunID)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, protocol.ErrInterruptNotOpen
	}
	if !s.claimSession(peek.SessionID) {
		return nil, nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, peek.SessionID)
	}
	defer s.releaseSession(peek.SessionID)
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
