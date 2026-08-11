package runtimeembedded

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type classifiedError struct {
	kind   error
	source error
}

func (e classifiedError) Error() string { return e.source.Error() }
func (e classifiedError) Unwrap() []error {
	return []error{e.kind, e.source}
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
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
			return classifiedError{kind: mapping.target, source: err}
		}
	}
	return err
}
