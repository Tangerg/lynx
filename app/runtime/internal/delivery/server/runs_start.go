package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	corechat "github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// StartRun translates runs.start into in-process execution
// path (API.md §7.3). It returns the runId synchronously; events flow
// out via the returned sequence as RunEvents (wrapped by the transport
// into notifications.run.event). The terminal segment.finished rides this
// sequence — including outcome:interrupt when the run parks for HITL
// approval, after which the run suspends and the client answers via
// runs.resume.
func (s *Server) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
	options := generationOptionsFromWire(in.Params)
	selection, err := modelref.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	input, err := decodeRunInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	// Negotiated before admission: the Run is created under this contract and keeps
	// it for life, so a capability we cannot honor has to stop the call rather than
	// be discovered halfway through its stream.
	capabilities, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.runs.Start(ctx, runs.StartCommand{
		SessionID:            in.SessionID,
		DefaultWorkspacePath: s.serverInfo.DefaultWorkspace.Path,
		ModelSelection:       selection,
		Limits: run.Limits{
			MaxTotalTokens: in.MaxTotalTokens,
			MaxSteps:       in.MaxSteps,
			MaxBudgetUSD:   in.MaxBudgetUSD,
		},
		Options:      options,
		Capabilities: capabilities,
		Input:        input,
	})
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	// Return the opening userMessage Item id so the client reconciles its
	// optimistic bubble by exact id (same id the stream + items.list carry).
	return &protocol.StartRunResponse{RunID: result.RunID, SegmentID: result.SegmentID, UserItemID: result.UserItemID}, mapRunEvents(ctx, result.Events), nil
}

func decodeRunInput(blocks []protocol.ContentBlock) ([]transcript.ContentBlock, error) {
	input := make([]transcript.ContentBlock, len(blocks))
	for i, block := range blocks {
		decoded, decodeErr := decodeContent(encodedContent{
			kind: block.Type, text: block.Text, mime: block.Mime, data: block.Data,
		})
		if decodeErr != nil {
			return nil, invalidWireContentBlock(i, decodeErr.field, decodeErr.detail)
		}
		input[i] = decoded
	}
	return input, nil
}

func invalidWireContentBlock(index int, field, detail string) error {
	constraint := &protocol.ConstraintError{Shape: "RunInput", Fields: []protocol.FieldError{{
		Field: fmt.Sprintf("input[%d].%s", index, field), Detail: detail,
	}}}
	return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, constraint)
}

func wireRunStartErr(err error) error {
	// A session that already has a run is refused WITH that run: the client offers
	// steer / resume / cancel, and the runtime cancels nothing on its own.
	if conflict, ok := errors.AsType[*runs.ActiveRunConflictError](err); ok {
		return &operation.ActiveRunConflictError{ActiveRun: protocol.ActiveRunRef{
			RunID: conflict.RunID, Status: presentRunStatus(conflict.Status),
		}}
	}
	switch {
	case errors.Is(err, runs.ErrInputRequired):
		return fmt.Errorf("%w: input must contain a user text or image block", protocol.ErrInvalidParams)
	case modelref.IsInvalid(err):
		return protocol.ErrInvalidParams
	case errors.Is(err, runs.ErrInvalidRunLimit):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrInvalidRunOptions):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrUnsupportedMedia):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrUnsupportedModelSelection):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrInvalidScheduledStart):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrSessionBusy):
		return protocol.ErrSessionBusy
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	default:
		return err
	}
}

func generationOptionsFromWire(in *protocol.GenerationParams) *corechat.Options {
	if in == nil {
		return nil
	}
	return &corechat.Options{
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
		TopP:        in.TopP,
		Stop:        slices.Clone(in.Stop),
	}
}
