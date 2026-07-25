package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

func newRunningTestState(ctx context.Context, handle TurnHandle, process agentexec.TurnProcess) *turnState {
	state := newPreparingTurnState(ctx, handle)
	state.completePreparation(runs.StartTurn{})
	if _, claimed := state.claimStart(); !claimed {
		panic("turn test fixture: prepared start was not claimable")
	}
	state.setProcess(process)
	return state
}
