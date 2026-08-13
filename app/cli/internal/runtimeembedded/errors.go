package runtimeembedded

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/failure"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
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

// runtimeContractViolation marks a response that cannot satisfy the protocol
// negotiated at startup. Callers may retry transport and storage failures, but
// the same malformed response cannot become valid through backoff.
func runtimeContractViolation(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", agent.ErrIncompatibleRuntime, fmt.Sprintf(format, arguments...))
}

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
		{protocol.ErrIdempotencyInProgress, agent.ErrCommandInProgress},
		{protocol.ErrIdempotencyConflict, agent.ErrCommandConflict},
		{protocol.ErrIdempotencyStoreMismatch, agent.ErrCommandStoreMismatch},
		{protocol.ErrCapabilityNotNeg, agent.ErrIncompatibleRuntime},
		{protocol.ErrInvalidProtocolVersion, agent.ErrIncompatibleRuntime},
		{protocol.ErrVcsUnavailable, workspace.ErrVersionControlUnavailable},
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
		if validationErr := problem.Validate(); validationErr != nil {
			return errors.Join(
				runtimeContractViolation("runtime problem is invalid: %v", validationErr),
				err,
			)
		}
	}
	if kind == nil && problem == nil {
		return err
	}
	return projectedError{kind: kind, source: err, problem: problem}
}
