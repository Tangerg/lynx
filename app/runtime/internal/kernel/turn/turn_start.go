package turn

import (
	"context"
	"errors"
)

func (s *inMemory) StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error) {
	if req.SessionID == "" {
		return TurnHandle{}, errors.New("turn: SessionID is required")
	}
	if err := req.Validate(); err != nil {
		return TurnHandle{}, err
	}

	handle := TurnHandle{
		SessionID: req.SessionID,
		TurnID:    newTurnID(),
	}
	state := newTurnState(ctx, handle)
	state.model = modelOr(req.Model)
	state.cwd = req.Cwd
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
		state.hooks = s.hooks.For(state.ctx, req.Cwd)
	}
	if !state.hooks.Empty() {
		msg, err := s.runPromptHooks(state.ctx, req, state)
		if err != nil {
			state.span.End()
			return TurnHandle{}, err
		}
		req.Message = msg
	}

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	go s.runTurn(req, state)

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
