package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ResumeRun answers an open interrupt by opening a NEW segment of the SAME run
// (R model, API.md §6). in.RunID is the stable run to continue; the response
// decision is delivered to the live agent process, and the continuation streams
// under the same runId with a fresh segmentId.
func (s *Server) ResumeRun(ctx context.Context, in protocol.ResumeRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	// Validate the decision BEFORE touching the interrupt — a malformed
	// response shouldn't consume the (still-resumable) record.
	responses, err := decodeResumeResponses(in.Responses)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.coordinator.Resume(ctx, runs.ResumeCommand{
		RunID:          in.RunID,
		Responses:      responses,
		InterruptKinds: interruptKindsFromContext(ctx),
	})
	if err != nil {
		switch {
		case errors.Is(err, runs.ErrInterruptNotOpen):
			return nil, nil, protocol.ErrInterruptNotOpen
		case errors.Is(err, runs.ErrInvalidInterruptResponse):
			return nil, nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
		case errors.Is(err, runs.ErrSessionBusy):
			return nil, nil, protocol.ErrSessionBusy
		case errors.Is(err, runs.ErrRunNotFound):
			return nil, nil, protocol.ErrRunNotFound
		default:
			return nil, nil, err
		}
	}
	return &protocol.StartRunResponse{RunID: result.RunID, SegmentID: result.SegmentID}, mapRunEvents(ctx, result.Events), nil
}

// decodeResumeResponses maps transport DTOs into the application-owned
// response union without looking up durable state. Exact item coverage,
// interrupt-kind matching, and question-schema validation belong to
// application/runs, where the open interrupt is available.
func decodeResumeResponses(responses []protocol.InterruptResponse) ([]runs.ResumeResponse, error) {
	out := make([]runs.ResumeResponse, 0, len(responses))
	for _, r := range responses {
		if r.ItemID == "" {
			return nil, fmt.Errorf("%w: interrupt response itemId is required", protocol.ErrInvalidParams)
		}
		response := runs.ResumeResponse{ItemID: r.ItemID}
		switch r.Response.Type {
		case protocol.InterruptResponseApproval:
			if r.Response.Answers != nil || r.Response.Result != nil || r.Response.Error != nil {
				return nil, fmt.Errorf("%w: approval response contains fields for another response type", protocol.ErrInvalidParams)
			}
			// remember{scope} persists this decision as a rule at the chosen
			// scope (session / project / global); the chat gate maps the scope
			// across and keys the rule (AUX_API §6). Empty = don't remember.
			approval := &runs.ApprovalResponse{Reason: r.Response.Reason}
			if r.Response.Remember != nil {
				scope, ok := rememberScopeFromWire(r.Response.Remember.Scope)
				if !ok {
					return nil, fmt.Errorf("%w: remember scope must be %q | %q | %q", protocol.ErrInvalidParams, protocol.RememberSession, protocol.RememberProject, protocol.RememberGlobal)
				}
				approval.RememberScope = scope
			}
			switch r.Response.Decision {
			case protocol.ApprovalApprove:
				approval.Approved = true
				// editedArgs overrides the model-regenerated tool args for this
				// one call (the gate's verdict.Arguments path). One-shot: never
				// folded into a remembered decision.
				if len(r.Response.EditedArgs) > 0 {
					b, err := json.Marshal(r.Response.EditedArgs)
					if err != nil {
						return nil, fmt.Errorf("runs.resume: editedArgs: %w", err)
					}
					approval.Arguments = string(b)
				}
			case protocol.ApprovalDeny:
				approval.Approved = false
			default:
				return nil, fmt.Errorf(`%w: approval decision must be "approve" | "deny"`, protocol.ErrInvalidParams)
			}
			response.Kind = runs.ApprovalResponseKind
			response.Approval = approval
		case protocol.InterruptResponseAnswer:
			if r.Response.Decision != "" || r.Response.Remember != nil || r.Response.EditedArgs != nil ||
				r.Response.Reason != "" || r.Response.Result != nil || r.Response.Error != nil {
				return nil, fmt.Errorf("%w: answer response contains fields for another response type", protocol.ErrInvalidParams)
			}
			// A question answer (ask_user / exit_plan_mode): the answers map IS
			// the resolution — the answering tool reads its chosen label / fields
			// back and decides what to do (e.g. exit_plan_mode treats a "Reject"
			// label as "stay in plan mode"). Always continues; the decision lives
			// in the answer content, not in Approved.
			response.Kind = runs.QuestionResponseKind
			response.Question = &runs.QuestionResponse{Answers: cloneWireAnswers(r.Response.Answers)}
		case protocol.InterruptResponseToolResult:
			return nil, fmt.Errorf("%w: toolResult does not answer a runtime interrupt", protocol.ErrInvalidParams)
		default:
			return nil, fmt.Errorf("%w: unknown interrupt response type %q", protocol.ErrInvalidParams, r.Response.Type)
		}
		out = append(out, response)
	}
	return out, nil
}

func cloneWireAnswers(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for name, values := range in {
		out[name] = append([]string(nil), values...)
	}
	return out
}
