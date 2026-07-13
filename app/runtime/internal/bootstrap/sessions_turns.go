package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// sessionsTurns adapts the agent turn dispatcher to the lifecycle coordinator's
// engine-neutral [sessions.Turns] port: it rebuilds a concrete turn handle from a
// [sessions.RunRef] and maps the dispatcher's resume errors onto the
// coordinator's neutral resume sentinels, so the application layer branches on
// resume semantics without importing the agent turn package.
type sessionsTurns struct {
	dispatcher turn.Dispatcher
}

func (t sessionsTurns) Cancel(ctx context.Context, ref sessions.RunRef) error {
	return t.dispatcher.Cancel(ctx, turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID})
}

func (t sessionsTurns) Prepare(ctx context.Context, ref sessions.RunRef) (sessions.Handle, error) {
	handle := turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID}
	if _, err := t.dispatcher.ProcessID(ctx, handle); err != nil {
		return nil, mapResumeError(err)
	}
	return handle, nil
}

func (t sessionsTurns) Resume(ctx context.Context, opaque sessions.Handle, resolution interrupts.Resolution, interruptKinds []string) error {
	handle, ok := opaque.(turn.TurnHandle)
	if !ok {
		return fmt.Errorf("bootstrap: resume handle %T is not a turn handle", opaque)
	}
	return mapResumeError(t.dispatcher.Resume(ctx, handle, resolution, interruptKinds))
}

func (t sessionsTurns) Rehydrate(ctx context.Context, req sessions.RehydrateSpec) (sessions.Handle, error) {
	handle, err := t.dispatcher.Rehydrate(ctx, turn.RehydrateRequest{
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		ProcessID: req.ProcessID,
		Provider:  req.Provider,
		Model:     req.Model,
	})
	if err != nil {
		return nil, mapResumeError(err)
	}
	return handle, nil
}

// mapResumeError translates the dispatcher's resume vocabulary into the
// coordinator's engine-neutral sentinels, preserving the original error in the
// chain so diagnostics keep the dispatcher's detail.
func mapResumeError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, turn.ErrParkClaimed):
		return fmt.Errorf("%w: %w", sessions.ErrParkClaimed, err)
	case errors.Is(err, turn.ErrTurnNotFound):
		return fmt.Errorf("%w: %w", sessions.ErrTurnNotLive, err)
	default:
		return err
	}
}
