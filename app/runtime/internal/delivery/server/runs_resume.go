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
			return nil, nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
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
	for _, wire := range responses {
		response, err := decodeResumeResponse(wire)
		if err != nil {
			return nil, err
		}
		out = append(out, response)
	}
	return out, nil
}

func decodeResumeResponse(wire protocol.InterruptResponse) (runs.ResumeResponse, error) {
	if wire.ItemID == "" {
		return runs.ResumeResponse{}, fmt.Errorf("%w: interrupt response itemId is required", protocol.ErrInvalidParams)
	}
	response := runs.ResumeResponse{ItemID: wire.ItemID}
	switch wire.Response.Type {
	case protocol.InterruptResponseApproval:
		approval, err := decodeApprovalResponse(wire.Response)
		if err != nil {
			return runs.ResumeResponse{}, err
		}
		response.Kind = runs.ApprovalResponseKind
		response.Approval = approval
	case protocol.InterruptResponseAnswer:
		question, err := decodeQuestionResponse(wire.Response)
		if err != nil {
			return runs.ResumeResponse{}, err
		}
		response.Kind = runs.QuestionResponseKind
		response.Question = question
	case protocol.InterruptResponseToolResult:
		return runs.ResumeResponse{}, fmt.Errorf("%w: toolResult does not answer a runtime interrupt", protocol.ErrInvalidParams)
	default:
		return runs.ResumeResponse{}, fmt.Errorf("%w: unknown interrupt response type %q", protocol.ErrInvalidParams, wire.Response.Type)
	}
	return response, nil
}

func decodeApprovalResponse(wire protocol.InterruptResponseValue) (*runs.ApprovalResponse, error) {
	if wire.Answers != nil || wire.Result != nil || wire.Error != nil {
		return nil, fmt.Errorf("%w: approval response contains fields for another response type", protocol.ErrInvalidParams)
	}
	// remember{scope} persists this decision as a rule at the chosen scope.
	// Empty means the one-shot decision is not remembered.
	approval := &runs.ApprovalResponse{Reason: wire.Reason}
	if wire.Remember != nil {
		scope, ok := rememberScopeFromWire(wire.Remember.Scope)
		if !ok {
			return nil, fmt.Errorf("%w: remember scope must be %q | %q | %q", protocol.ErrInvalidParams, protocol.RememberSession, protocol.RememberProject, protocol.RememberGlobal)
		}
		approval.RememberScope = scope
	}
	switch wire.Decision {
	case protocol.ApprovalApprove:
		approval.Approved = true
		if len(wire.EditedArgs) > 0 {
			encoded, err := json.Marshal(wire.EditedArgs)
			if err != nil {
				return nil, fmt.Errorf("runs.resume: editedArgs: %w", err)
			}
			approval.Arguments = string(encoded)
		}
	case protocol.ApprovalDeny:
		approval.Approved = false
	default:
		return nil, fmt.Errorf(`%w: approval decision must be "approve" | "deny"`, protocol.ErrInvalidParams)
	}
	return approval, nil
}

func decodeQuestionResponse(wire protocol.InterruptResponseValue) (*runs.QuestionResponse, error) {
	if wire.Decision != "" || wire.Remember != nil || wire.EditedArgs != nil || wire.Reason != "" || wire.Result != nil || wire.Error != nil {
		return nil, fmt.Errorf("%w: answer response contains fields for another response type", protocol.ErrInvalidParams)
	}
	// The answer map is the complete question resolution. The application later
	// validates it against the stored question's field schema.
	return &runs.QuestionResponse{Answers: cloneWireAnswers(wire.Answers)}, nil
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
