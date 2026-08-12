package runtimeembedded

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/failure"
)

type projectedError struct {
	kind    error
	source  error
	problem *failure.Problem
}

func (e projectedError) Error() string {
	if e.problem != nil {
		return e.problem.String()
	}
	return e.source.Error()
}

func (e projectedError) Unwrap() []error {
	if e.kind == nil {
		return []error{e.source}
	}
	return []error{e.kind, e.source}
}

func (e projectedError) Failure() *failure.Problem { return e.problem.Clone() }

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var kind error
	for _, mapping := range []struct {
		source error
		target error
	}{
		{protocol.ErrSessionNotFound, agent.ErrSessionNotFound},
		{protocol.ErrRunNotFound, agent.ErrRunNotFound},
		{protocol.ErrInterruptNotOpen, agent.ErrInterruptNotOpen},
		{protocol.ErrStaleSegment, agent.ErrStaleSegment},
		{protocol.ErrRunWaiting, agent.ErrRunWaiting},
		{protocol.ErrRunFinished, agent.ErrRunFinished},
		{protocol.ErrReplayCursorInvalid, agent.ErrReplayCursorInvalid},
		{protocol.ErrReplayUnavailable, agent.ErrReplayUnavailable},
		{protocol.ErrSessionHasActiveRun, agent.ErrSessionHasActiveRun},
		{protocol.ErrSessionBusy, agent.ErrSessionBusy},
		{protocol.ErrRevisionConflict, agent.ErrRevisionConflict},
		{protocol.ErrCapabilityNotNeg, agent.ErrIncompatibleRuntime},
		{protocol.ErrInvalidProtocolVersion, agent.ErrIncompatibleRuntime},
		{embedded.ErrClosed, agent.ErrDisconnected},
	} {
		if errors.Is(err, mapping.source) {
			kind = mapping.target
			break
		}
	}
	var problem *failure.Problem
	if source, ok := errors.AsType[protocol.ProblemError](err); ok {
		data := source.Problem()
		problem = projectRuntimeProblem(&data)
	}
	if kind == nil && problem == nil {
		return err
	}
	return projectedError{kind: kind, source: err, problem: problem}
}
