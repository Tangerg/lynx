package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// CancelRun hard-stops a running run (outcome:canceled, API.md §7.3).
// A parked Run is also abandoned — its live parked execution is torn down
// and its open interrupt dropped so it stops surfacing as resumable.
func (s *Server) CancelRun(ctx context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	// Root cancel is the emergency stop and is always allowed. Whether the target
	// is a child is durable state resolved by the application, so do not reject
	// unsupported client preferences before that identity is known.
	result, err := s.runs.Cancel(ctx, runs.CancelCommand{
		RunID:         in.RunID,
		Reason:        in.Reason,
		AllowChildRun: s.requestCanUseFeature(ctx, protocol.FeatureSubagents),
	})
	switch {
	case errors.Is(err, runs.ErrRunNotFound):
		return nil, protocol.ErrRunNotFound
	case errors.Is(err, runs.ErrRunFinished):
		return nil, protocol.ErrRunFinished
	case errors.Is(err, runs.ErrSessionBusy):
		return nil, protocol.ErrSessionBusy
	case errors.Is(err, runs.ErrChildRunNotAllowed):
		return nil, protocol.NewCapabilityGapError(protocol.CapabilityRequirement{
			Type: protocol.RequirementFeature,
			Name: protocol.FeatureSubagents,
		})
	case err != nil:
		return nil, err
	default:
		return presentCancelResult(result), nil
	}
}

// SteerRun injects a user message into the segment the request names so the model
// reads it on its next tool round (runs.steer, API.md §6).
//
// Only the addressed segment is steerable, and every other position says so by
// name: a waiting run is answered via runs.resume, a finished one cannot be
// steered at all, and a run that has moved to a different segment refuses rather
// than delivering the instruction to work the user never saw. The refusals are
// the same set a subscribe gets, because both are addressing one live segment.
func (s *Server) SteerRun(ctx context.Context, in protocol.SteerRunRequest) error {
	input, err := decodeRunInput(in.Input)
	if err != nil {
		return err
	}
	return wireSteerError(s.runs.Steer(ctx, runs.SteerCommand{
		RunID: in.RunID, ExpectedSegmentID: in.ExpectedSegmentID, Input: input,
	}))
}

func wireSteerError(err error) error {
	if input, ok := errors.AsType[*runs.InputBlockError](err); ok {
		constraint := &protocol.ConstraintError{Shape: "RunInput", Fields: []protocol.FieldError{{
			Field:  fmt.Sprintf("input[%d].%s", input.Index, input.Field),
			Detail: input.Detail,
		}}}
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, constraint)
	}
	if errors.Is(err, runs.ErrInputRequired) || errors.Is(err, runs.ErrUnsupportedMedia) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return wireLiveSegmentError(err)
}
