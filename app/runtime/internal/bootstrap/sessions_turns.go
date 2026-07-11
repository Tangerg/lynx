package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
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

func (t sessionsTurns) Resume(ctx context.Context, ref sessions.RunRef, resolution interrupts.Resolution, interruptKinds []string) (sessions.Handle, error) {
	handle := turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID}
	if err := t.dispatcher.Resume(ctx, handle, resolution, interruptKinds); err != nil {
		return nil, mapResumeError(err)
	}
	return handle, nil
}

func (t sessionsTurns) Rehydrate(ctx context.Context, req sessions.RehydrateSpec) (sessions.Handle, error) {
	handle, err := t.dispatcher.Rehydrate(ctx, turn.RehydrateRequest{
		SessionID:      req.SessionID,
		ProcessID:      req.ProcessID,
		Approved:       req.Approved,
		Provider:       req.Provider,
		Model:          req.Model,
		InterruptKinds: req.InterruptKinds,
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
	switch {
	case errors.Is(err, turn.ErrParkClaimed):
		return fmt.Errorf("%w: %w", sessions.ErrParkClaimed, err)
	case errors.Is(err, turn.ErrTurnNotFound):
		return fmt.Errorf("%w: %w", sessions.ErrTurnNotLive, err)
	case errors.Is(err, turn.ErrRehydrateCommitted):
		return fmt.Errorf("%w: %w", sessions.ErrRehydrateCommitted, err)
	default:
		return err
	}
}
