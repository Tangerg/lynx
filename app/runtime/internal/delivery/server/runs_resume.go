package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// ResumeRun answers an open interrupt by opening a NEW segment of the SAME run
// (R model, API.md §6). in.RunID is the stable run to continue; the response
// decision is delivered to the live agent process, and the continuation streams
// under the same runId with a fresh segmentId.
func (s *Server) ResumeRun(ctx context.Context, in protocol.ResumeRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	// Validate the decision BEFORE touching the interrupt — a malformed
	// response shouldn't consume the (still-resumable) record.
	resolution, err := resolveResolution(in.Responses)
	if err != nil {
		return nil, nil, err
	}
	pending, admission, err := s.sessions.ClaimResumeSlot(ctx, s.coordinator, in.RunID)
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrInterruptNotOpen):
			return nil, nil, protocol.ErrInterruptNotOpen
		case errors.Is(err, sessions.ErrSessionBusy):
			return nil, nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, pending.SessionID)
		default:
			return nil, nil, err
		}
	}
	defer admission.Release()

	sess, err := s.sessions.Get(ctx, pending.SessionID)
	if err != nil {
		return nil, nil, wireSessionErr(err)
	}
	treeAdmission, ok := s.sessions.ClaimWorkingTreeRun(sess.Cwd)
	if !ok {
		return nil, nil, fmt.Errorf("%w: working tree %q has a file restore in flight", protocol.ErrSessionBusy, sess.Cwd)
	}
	releaseTreeAdmission := true
	defer func() {
		if releaseTreeAdmission {
			treeAdmission.Release()
		}
	}()

	resumed, err := s.sessions.ResumeClaimedInterrupt(ctx, in.RunID, resolution, interruptKindsFromContext(ctx))
	if err != nil {
		switch {
		case errors.Is(err, sessions.ErrInterruptNotOpen):
			return nil, nil, protocol.ErrInterruptNotOpen
		case errors.Is(err, sessions.ErrRunNotFound):
			return nil, nil, protocol.ErrRunNotFound
		default:
			return nil, nil, err
		}
	}
	pending = resumed.Pending
	handle, ok := resumed.Handle.(turn.TurnHandle)
	if !ok {
		return nil, nil, fmt.Errorf("resume: executor handle %T is not a turn handle", resumed.Handle)
	}

	// A resume opens a NEW segment of the SAME run: the runId (in.RunID = the
	// stable logical run) is unchanged, and a fresh segmentId identifies this
	// continuation stream. handle.TurnID is the executor's turn handle (= the
	// ProcessID) — distinct from the stable runId.
	segmentID := protocol.IDPrefixSegment + uuid.NewString()
	// A continuation carries no new user turn — the decision is delivered
	// out-of-band via runs.resume, so no opening userMessage Item. It DOES
	// carry the resume binding: an approved tool re-fires in this segment and
	// must complete its ORIGINAL proposal item (API.md §5.2 / §6), not a fresh
	// one. It runs against the parked run's same provider+model.
	//
	// createdAt is the RUN's original start time (carried on the interrupt), NOT
	// now: the run's segments share one durable transcript record keyed by the
	// stable runId, so a resume that terminates must not shift the run's timeline
	// position (it anchors rollback/fork ordering + subagent grouping, §10.3).
	createdAt := pending.RunCreatedAt
	factory := s.segmentProjector(in.RunID, segmentID, pending.SessionID, sess.Cwd, handle, nil, resumeBindingFrom(pending), pending.Provider, pending.Model, createdAt)
	evCh, err := s.coordinator.Start(ctx, runs.StartSpec{
		RunID:     in.RunID,
		SegmentID: segmentID,
		Resume:    true,
		SessionID: pending.SessionID,
		Cwd:       worktree.CanonicalCwd(sess.Cwd),
		TurnID:    handle.TurnID,
		Handle:    handle,
		Provider:  pending.Provider,
		Model:     pending.Model,
		CreatedAt: createdAt,
	}, factory)
	if err != nil {
		// The interrupt was already consumed and the parked turn resumed; a Start
		// failure (Coordinator closing / executor error) would otherwise strand the
		// session with a non-terminal run and no interrupt to resume. Re-open the
		// interrupt so a retry can resume it — Start already canceled the turn, and a
		// later resume rehydrates a fresh one from the durable snapshot.
		_ = s.sessions.RestoreConsumedInterrupt(ctx, pending)
		return nil, nil, err
	}
	treeAdmission.Release()
	releaseTreeAdmission = false
	return &protocol.StartRunResponse{RunID: in.RunID, SegmentID: segmentID}, mapRunEvents(ctx, evCh), nil
}

// resolveResolution maps the wire interrupt responses onto the structured
// [interrupts.Resolution] the turn dispatcher's Resume expects. The agent
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
