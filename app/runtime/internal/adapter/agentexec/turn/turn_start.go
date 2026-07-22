package turn

import (
	"context"
	"errors"
	"fmt"
)

func (s *memoryDispatcher) StartTurn(ctx context.Context, request StartTurnRequest) (TurnHandle, error) {
	handle, err := s.PrepareTurn(ctx, request)
	if err != nil {
		return TurnHandle{}, err
	}
	if err := s.ActivateTurn(ctx, handle); err != nil {
		_ = s.Cancel(context.WithoutCancel(ctx), handle)
		return TurnHandle{}, err
	}
	return handle, nil
}

// PrepareTurn establishes all reversible turn state but deliberately does not
// launch the engine. The application can now durably admit its Run before
// ActivateTurn crosses the model/tool side-effect boundary.
func (s *memoryDispatcher) PrepareTurn(ctx context.Context, request StartTurnRequest) (TurnHandle, error) {
	if request.SessionID == "" {
		return TurnHandle{}, errors.New("turn: SessionID is required")
	}
	request = request.snapshot()
	if err := request.Validate(); err != nil {
		return TurnHandle{}, err
	}
	if s.isClosed() {
		return TurnHandle{}, ErrDispatcherClosed
	}

	handle := TurnHandle{
		SessionID: request.SessionID,
		TurnID:    newTurnID(),
	}
	state := newTurnState(ctx, handle)
	handle.state = state
	state.model = modelOr(request.Model)
	state.provider = request.Provider
	state.cwd = request.Cwd
	state.setInterruptKinds(request.InterruptKinds)
	// Open the turn span synchronously (before the goroutine launches and
	// before the handle is returned) so st.ctx carries it for every later
	// reader — runTurn, drive, resume, Cancel. The entry trace rode in via
	// newTurnState's WithoutCancel, so this span is its child.
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)

	// Resolve this turn's lifecycle hooks (trust-filtered for the cwd). The
	// UserPromptSubmit / SessionStart hooks run BEFORE the turn launches so they
	// can inject context into the prompt or block it; a block ends the span we
	// just opened and fails the start.
	if s.hooks != nil {
		resolved, err := s.hooks.For(state.ctx, request.Cwd)
		if err != nil {
			state.cancel()
			state.span.RecordError(err)
			state.span.End()
			return TurnHandle{}, fmt.Errorf("turn: resolve lifecycle hooks: %w", err)
		}
		state.hooks = resolved
	}
	if !state.hooks.Empty() {
		msg, err := s.runPromptHooks(state.ctx, request, state)
		if err != nil {
			state.cancel()
			state.span.RecordError(err)
			state.span.End()
			return TurnHandle{}, err
		}
		request.Message = msg
	}
	// Capture the request AFTER the prompt hooks so the (possibly context-injected)
	// message is what Activate replays into the turn; prepareStart before the hooks
	// would snapshot the pre-injection prompt and silently drop UserPromptSubmit /
	// SessionStart InjectContext.
	state.prepareStart(request)

	if !s.register(state) {
		state.cancel()
		state.span.End()
		return TurnHandle{}, ErrDispatcherClosed
	}

	return handle, nil
}

// ActivateTurn launches a prepared turn exactly once.
func (s *memoryDispatcher) ActivateTurn(_ context.Context, handle TurnHandle) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	request, ok := state.claimStart()
	if !ok {
		return ErrTurnAlreadyActivated
	}
	go s.runTurn(request, state)
	return nil
}

// modelOr returns the model name for display / observability, falling
// back to "default" when the turn didn't pick one.
func modelOr(model string) string {
	if model == "" {
		return "default"
	}
	return model
}
