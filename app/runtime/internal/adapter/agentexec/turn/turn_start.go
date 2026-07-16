package turn

import (
	"context"
	"errors"
)

func (s *memoryDispatcher) StartTurn(ctx context.Context, request StartTurnRequest) (TurnHandle, error) {
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
		state.hooks = s.hooks.For(state.ctx, request.Cwd)
	}
	if !state.hooks.Empty() {
		msg, err := s.runPromptHooks(state.ctx, request, state)
		if err != nil {
			state.span.End()
			return TurnHandle{}, err
		}
		request.Message = msg
	}

	if !s.register(state) {
		state.cancel()
		state.span.End()
		return TurnHandle{}, ErrDispatcherClosed
	}

	go s.runTurn(request, state)

	return handle, nil
}

// modelOr returns the model name for display / observability, falling
// back to "default" when the turn didn't pick one.
func modelOr(model string) string {
	if model == "" {
		return "default"
	}
	return model
}
