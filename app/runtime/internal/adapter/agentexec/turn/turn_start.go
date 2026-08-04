package turn

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

func (s *controller) StartTurn(ctx context.Context, request runs.StartExecution) (Handle, error) {
	handle, err := s.PrepareTurn(ctx, request)
	if err != nil {
		return Handle{}, err
	}
	if err := s.ActivateTurn(ctx, handle); err != nil {
		cleanupErr := s.Cancel(ctx, handle)
		if errors.Is(cleanupErr, ErrTurnNotFound) {
			cleanupErr = nil
		}
		return Handle{}, errors.Join(err, cleanupErr)
	}
	return handle, nil
}

// PrepareTurn establishes all reversible turn state but deliberately does not
// launch the engine. The application can now durably admit its Run before
// ActivateTurn crosses the model/tool side-effect boundary.
func (s *controller) PrepareTurn(ctx context.Context, request runs.StartExecution) (Handle, error) {
	if request.SessionID == "" {
		return Handle{}, errors.New("turn: SessionID is required")
	}
	request = snapshotStartTurn(request)
	if err := request.Validate(); err != nil {
		return Handle{}, err
	}
	if request.ModelSelection.Configured() && s.resolver == nil {
		return Handle{}, errors.New("turn: explicit model selection requires a client resolver")
	}
	if s.isClosed() {
		return Handle{}, ErrClosed
	}

	handle := Handle{
		SessionID: request.SessionID,
		TurnID:    newTurnID(),
	}
	state := newPreparingTurnState(ctx, handle)
	handle.state = state
	state.model = modelOr(request.ModelSelection.Model())
	state.modelSelection = request.ModelSelection
	state.cwd = request.Cwd
	state.setInterruptKinds(request.InterruptKinds)
	// Open the turn span synchronously (before the goroutine launches and
	// before the handle is returned) so st.ctx carries it for every later
	// reader — runTurn, drive, resume, Cancel. The entry trace rode in via
	// the state constructor's WithoutCancel, so this span is its child.
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
			return Handle{}, fmt.Errorf("turn: resolve lifecycle hooks: %w", err)
		}
		state.hooks = resolved
	}
	if !state.hooks.Empty() {
		msg, err := s.runPromptHooks(state.ctx, request, state)
		if err != nil {
			state.cancel()
			state.span.RecordError(err)
			state.span.End()
			return Handle{}, err
		}
		request.Message = msg
	}
	// Capture the request AFTER the prompt hooks so the (possibly context-injected)
	// message is what Activate replays into the turn; completing preparation before the hooks
	// would snapshot the pre-injection prompt and silently drop UserPromptSubmit /
	// SessionStart InjectContext.
	state.completePreparation(request)

	if !s.register(state) {
		state.cancel()
		state.span.End()
		return Handle{}, ErrClosed
	}

	return handle, nil
}

// ActivateTurn launches a prepared turn exactly once.
func (s *controller) ActivateTurn(_ context.Context, handle Handle) error {
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
